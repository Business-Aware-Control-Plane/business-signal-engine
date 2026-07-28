package provider

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/option"
)

type GoogleAnalyticsProvider struct {
	cfg *config.Config
}

func NewGoogleAnalyticsProvider(cfg *config.Config) *GoogleAnalyticsProvider {
	return &GoogleAnalyticsProvider{cfg: cfg}
}

func (p *GoogleAnalyticsProvider) Name() string {
	return "GoogleAnalytics"
}

func (p *GoogleAnalyticsProvider) PollFrequency() time.Duration {
	return 5 * time.Minute
}

func (p *GoogleAnalyticsProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	if p.cfg.GAPropertyID == "" {
		log.Printf("[WARN] [GoogleAnalytics] GA_PROPERTY_ID is not configured. Skipping extraction to prevent fake data in database.")
		return nil, nil
	}

	// Create authenticated OAuth2 / Service client if tokens are available
	client, err := p.createGAClient(ctx)
	if err != nil {
		log.Printf("[WARN] [GoogleAnalytics] Client authentication failed: %v. Skipping extraction.", err)
		return nil, nil
	}

	log.Printf("[INFO] [GoogleAnalytics] Executing GA4 Data API reporting for Property ID: %s", p.cfg.GAPropertyID)

	// 1. Fetch Realtime Metrics (activeUsers, screenPageViews, eventCount)
	realtimeSignals, err := p.fetchRealtimeReport(ctx, client)
	if err != nil {
		log.Printf("[WARN] [GoogleAnalytics] Realtime report error: %v", err)
	}

	// 2. Fetch Core Funnel & Engagement Metrics (engagementRate, keyEvents, checkoutCount)
	coreSignals, err := p.fetchCoreReport(ctx, client)
	if err != nil {
		log.Printf("[WARN] [GoogleAnalytics] Core report error: %v", err)
	}

	var allSignals []model.Signal
	allSignals = append(allSignals, realtimeSignals...)
	allSignals = append(allSignals, coreSignals...)

	log.Printf("[INFO] [GoogleAnalytics] Successfully extracted %d GA4 signals for property %s", len(allSignals), p.cfg.GAPropertyID)
	return allSignals, nil
}

func (p *GoogleAnalyticsProvider) createGAClient(ctx context.Context) (*analyticsdata.Service, error) {
	if p.cfg.GoogleClientID != "" && p.cfg.GoogleRefreshToken != "" {
		oConfig := &oauth2.Config{
			ClientID:     p.cfg.GoogleClientID,
			ClientSecret: p.cfg.GoogleClientSecret,
			Scopes:       []string{"https://www.googleapis.com/auth/analytics.readonly", "https://www.googleapis.com/auth/analytics"},
			Endpoint: oauth2.Endpoint{
				TokenURL: "https://oauth2.googleapis.com/token",
			},
		}
		token := &oauth2.Token{
			RefreshToken: p.cfg.GoogleRefreshToken,
		}
		tokenSource := oConfig.TokenSource(ctx, token)
		return analyticsdata.NewService(ctx, option.WithTokenSource(tokenSource))
	}

	// Fallback to Application Default Credentials (ADC)
	return analyticsdata.NewService(ctx)
}

func (p *GoogleAnalyticsProvider) fetchRealtimeReport(ctx context.Context, service *analyticsdata.Service) ([]model.Signal, error) {
	propertyRes := fmt.Sprintf("properties/%s", p.cfg.GAPropertyID)

	req := &analyticsdata.RunRealtimeReportRequest{
		Metrics: []*analyticsdata.Metric{
			{Name: "activeUsers"},
			{Name: "screenPageViews"},
			{Name: "eventCount"},
			{Name: "keyEvents"},
		},
	}

	resp, err := service.Properties.RunRealtimeReport(propertyRes, req).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("RunRealtimeReport failed: %w", err)
	}

	now := time.Now()
	var signals []model.Signal

	if resp != nil && len(resp.Rows) > 0 {
		row := resp.Rows[0]
		metricNames := []string{"active_users", "screen_page_views", "event_count", "key_events"}
		units := []string{"count", "count", "count", "count"}

		for i, metricVal := range row.MetricValues {
			if i >= len(metricNames) {
				break
			}
			val := parseMetricValue(metricVal.Value)
			signals = append(signals, model.Signal{
				SignalID:   uuid.New().String(),
				Source:     "google_analytics",
				Type:       metricNames[i],
				Value:      val,
				Unit:       units[i],
				Confidence: 0.98,
				Metadata: map[string]interface{}{
					"propertyId": p.cfg.GAPropertyID,
					"apiType":    "realtime",
				},
				Timestamp: now,
			})
		}
	}

	return signals, nil
}

func (p *GoogleAnalyticsProvider) fetchCoreReport(ctx context.Context, service *analyticsdata.Service) ([]model.Signal, error) {
	propertyRes := fmt.Sprintf("properties/%s", p.cfg.GAPropertyID)

	req := &analyticsdata.RunReportRequest{
		DateRanges: []*analyticsdata.DateRange{
			{StartDate: "today", EndDate: "today"},
		},
		Metrics: []*analyticsdata.Metric{
			{Name: "engagementRate"},
			{Name: "userEngagementDuration"},
			{Name: "bounceRate"},
			{Name: "conversions"},
		},
	}

	resp, err := service.Properties.RunReport(propertyRes, req).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("RunReport failed: %w", err)
	}

	now := time.Now()
	var signals []model.Signal

	if resp != nil && len(resp.Rows) > 0 {
		row := resp.Rows[0]
		metricNames := []string{"engagement_rate", "user_engagement_duration", "bounce_rate", "conversions"}
		units := []string{"percentage", "seconds", "percentage", "count"}

		for i, metricVal := range row.MetricValues {
			if i >= len(metricNames) {
				break
			}
			val := parseMetricValue(metricVal.Value)
			signals = append(signals, model.Signal{
				SignalID:   uuid.New().String(),
				Source:     "google_analytics",
				Type:       metricNames[i],
				Value:      val,
				Unit:       units[i],
				Confidence: 0.95,
				Metadata: map[string]interface{}{
					"propertyId": p.cfg.GAPropertyID,
					"apiType":    "core_report",
				},
				Timestamp: now,
			})
		}
	}

	return signals, nil
}

func parseMetricValue(valStr string) float64 {
	var val float64
	fmt.Sscanf(valStr, "%f", &val)
	return val
}
