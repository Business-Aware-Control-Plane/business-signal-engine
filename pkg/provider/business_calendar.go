package provider

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// BusinessCalendarProvider reads a dedicated marketing/ops calendar (product
// launches, campaign windows) — distinct from CalendarProvider, which only
// answers "is today a public holiday" via Nager.Date. This closes the other
// half of BSAL-HB-01 §04: the FigJam design's "Email Campaign" / "Product
// Launch" boxes previously had no supporting provider at all.
type BusinessCalendarProvider struct {
	cfg *config.Config
}

func NewBusinessCalendarProvider(cfg *config.Config) *BusinessCalendarProvider {
	return &BusinessCalendarProvider{cfg: cfg}
}

func (p *BusinessCalendarProvider) Name() string { return "BusinessCalendar" }

func (p *BusinessCalendarProvider) PollFrequency() time.Duration {
	return pollFrequencyOr(p.cfg.BusinessCalendarPollInterval, 30*time.Minute)
}

func (p *BusinessCalendarProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	if p.cfg.BusinessCalendarID == "" {
		log.Printf("[WARN] [BusinessCalendar] BUSINESS_CALENDAR_ID is not configured. Skipping extraction to prevent fake data in database.")
		return nil, nil
	}

	srv, err := p.createCalendarClient(ctx)
	if err != nil {
		log.Printf("[WARN] [BusinessCalendar] Client authentication failed: %v. Skipping extraction.", err)
		return nil, nil
	}

	now := time.Now()
	horizon := now.Add(7 * 24 * time.Hour)

	events, err := srv.Events.List(p.cfg.BusinessCalendarID).
		TimeMin(now.Add(-1 * time.Hour).Format(time.RFC3339)).
		TimeMax(horizon.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("calendar Events.List failed: %w", err)
	}

	var signals []model.Signal
	for _, ev := range events.Items {
		start, end, ok := eventWindow(ev)
		if !ok {
			continue
		}

		eventType := classifyBusinessEvent(ev)
		meta := map[string]interface{}{
			"summary":    ev.Summary,
			"start":      start.Format(time.RFC3339),
			"end":        end.Format(time.RFC3339),
			"calendarId": p.cfg.BusinessCalendarID,
		}

		switch {
		case !now.Before(start) && !now.After(end):
			signals = append(signals, model.Signal{
				SignalID:   uuid.New().String(),
				Source:     "business_calendar",
				Type:       eventType,
				Value:      1,
				Unit:       "boolean",
				Confidence: 1.0,
				Metadata:   meta,
				Timestamp:  now,
			})
		case start.After(now) && start.Before(horizon):
			metaUpcoming := map[string]interface{}{}
			for k, v := range meta {
				metaUpcoming[k] = v
			}
			metaUpcoming["startsInHours"] = start.Sub(now).Hours()
			signals = append(signals, model.Signal{
				SignalID:   uuid.New().String(),
				Source:     "business_calendar",
				Type:       eventType + "_upcoming",
				Value:      1,
				Unit:       "boolean",
				Confidence: 1.0, // scheduled events are known facts, not estimates
				Metadata:   metaUpcoming,
				Timestamp:  now,
			})
		}
	}

	log.Printf("[INFO] [BusinessCalendar] Extracted %d business calendar signals from %s", len(signals), p.cfg.BusinessCalendarID)
	return signals, nil
}

// classifyBusinessEvent prefers an explicit tag (organisers set
// extendedProperties.shared.businessEventType on the calendar entry) over a
// keyword heuristic on the title, since a tag is unambiguous and a title is not.
func classifyBusinessEvent(ev *calendar.Event) string {
	if ev.ExtendedProperties != nil && ev.ExtendedProperties.Shared != nil {
		if t, ok := ev.ExtendedProperties.Shared["businessEventType"]; ok && t != "" {
			return t
		}
	}
	summary := strings.ToLower(ev.Summary)
	switch {
	case strings.Contains(summary, "launch"):
		return "product_launch"
	case strings.Contains(summary, "campaign") || strings.Contains(summary, "sale") || strings.Contains(summary, "promo"):
		return "campaign_window"
	default:
		return "campaign_window"
	}
}

// eventWindow normalises a calendar.Event's Start/End, which the API
// expresses as either an all-day Date (YYYY-MM-DD) or a timed DateTime
// (RFC3339), into two concrete times.
func eventWindow(ev *calendar.Event) (start, end time.Time, ok bool) {
	if ev.Start == nil || ev.End == nil {
		return time.Time{}, time.Time{}, false
	}
	s, sOK := parseEventDateTime(ev.Start)
	e, eOK := parseEventDateTime(ev.End)
	if !sOK || !eOK {
		return time.Time{}, time.Time{}, false
	}
	return s, e, true
}

func parseEventDateTime(dt *calendar.EventDateTime) (time.Time, bool) {
	if dt.DateTime != "" {
		t, err := time.Parse(time.RFC3339, dt.DateTime)
		return t, err == nil
	}
	if dt.Date != "" {
		t, err := time.Parse("2006-01-02", dt.Date)
		return t, err == nil
	}
	return time.Time{}, false
}

func (p *BusinessCalendarProvider) createCalendarClient(ctx context.Context) (*calendar.Service, error) {
	var opts []option.ClientOption
	if p.cfg.CalendarAPIEndpoint != "" {
		// Test-only seam, mirrors GoogleAnalyticsProvider's GAEndpoint override.
		opts = append(opts, option.WithEndpoint(p.cfg.CalendarAPIEndpoint), option.WithoutAuthentication())
		return calendar.NewService(ctx, opts...)
	}

	if p.cfg.GoogleClientID != "" && p.cfg.GoogleRefreshToken != "" {
		oConfig := &oauth2.Config{
			ClientID:     p.cfg.GoogleClientID,
			ClientSecret: p.cfg.GoogleClientSecret,
			Scopes:       []string{"https://www.googleapis.com/auth/calendar.readonly"},
			Endpoint: oauth2.Endpoint{
				TokenURL: "https://oauth2.googleapis.com/token",
			},
		}
		token := &oauth2.Token{RefreshToken: p.cfg.GoogleRefreshToken}
		return calendar.NewService(ctx, option.WithTokenSource(oConfig.TokenSource(ctx, token)))
	}

	return calendar.NewService(ctx)
}
