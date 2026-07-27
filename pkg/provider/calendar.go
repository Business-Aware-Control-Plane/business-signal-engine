package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
)

type CalendarProvider struct {
	cfg        *config.Config
	httpClient *http.Client
}

type nagerHoliday struct {
	Date        string   `json:"date"`
	LocalName   string   `json:"localName"`
	Name        string   `json:"name"`
	CountryCode string   `json:"countryCode"`
	Types       []string `json:"types"`
}

func NewCalendarProvider(cfg *config.Config) *CalendarProvider {
	return &CalendarProvider{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *CalendarProvider) Name() string {
	return "Calendar"
}

func (p *CalendarProvider) PollFrequency() time.Duration {
	return 30 * time.Minute // Poll calendar every 30 minutes
}

func (p *CalendarProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	now := time.Now()
	year := now.Year()
	url := fmt.Sprintf("https://date.nager.at/api/v3/PublicHolidays/%d/%s", year, p.cfg.CountryCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("[WARN] Calendar API error, fallback to simulated holiday signal: %v", err)
		return p.generateMockSignals(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARN] Calendar API status %d, fallback to simulated holiday signal", resp.StatusCode)
		return p.generateMockSignals(), nil
	}

	var holidays []nagerHoliday
	if err := json.NewDecoder(resp.Body).Decode(&holidays); err != nil {
		return nil, fmt.Errorf("failed to decode holiday calendar response: %w", err)
	}

	todayStr := now.Format("2006-01-02")
	isTodayHoliday := false
	var todayHolidayName string
	upcomingHolidaysCount := 0

	sevenDaysLater := now.AddDate(0, 0, 7)

	for _, h := range holidays {
		if h.Date == todayStr {
			isTodayHoliday = true
			todayHolidayName = h.Name
		}
		hTime, err := time.Parse("2006-01-02", h.Date)
		if err == nil {
			if hTime.After(now) && hTime.Before(sevenDaysLater) {
				upcomingHolidaysCount++
			}
		}
	}

	holidayValue := 0.0
	if isTodayHoliday {
		holidayValue = 1.0
	}

	signals := []model.Signal{
		{
			SignalID:   uuid.New().String(),
			Source:     "calendar",
			Type:       "public_holiday",
			Value:      holidayValue,
			Unit:       "boolean",
			Confidence: 1.0,
			Metadata: map[string]interface{}{
				"country":        p.cfg.CountryCode,
				"isHoliday":      isTodayHoliday,
				"holidayName":    todayHolidayName,
				"upcomingCount":  upcomingHolidaysCount,
			},
			Timestamp: now,
		},
		{
			SignalID:   uuid.New().String(),
			Source:     "calendar",
			Type:       "upcoming_holidays_7d",
			Value:      float64(upcomingHolidaysCount),
			Unit:       "count",
			Confidence: 0.95,
			Metadata: map[string]interface{}{
				"country": p.cfg.CountryCode,
			},
			Timestamp: now,
		},
	}

	log.Printf("[INFO] [Calendar] Extracted %d signals for %s (IsTodayHoliday: %v, TodayHoliday: '%s')", len(signals), p.cfg.CountryCode, isTodayHoliday, todayHolidayName)
	return signals, nil
}

func (p *CalendarProvider) generateMockSignals() []model.Signal {
	now := time.Now()
	return []model.Signal{
		{
			SignalID:   uuid.New().String(),
			Source:     "calendar",
			Type:       "public_holiday",
			Value:      0.0,
			Unit:       "boolean",
			Confidence: 0.9,
			Metadata: map[string]interface{}{
				"country":   p.cfg.CountryCode,
				"simulated": true,
			},
			Timestamp: now,
		},
	}
}
