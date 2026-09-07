package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

func TestCalendarProvider_Success_TodayIsHoliday(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	p := provider.NewCalendarProvider(&config.Config{CountryCode: "LK"})
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals (public_holiday, upcoming_holidays_7d), got %d", len(signals))
	}
	for _, s := range signals {
		if s.Type == "public_holiday" && s.Value != 1.0 {
			t.Errorf("expected today's fixture holiday to set public_holiday=1, got %.0f", s.Value)
		}
	}
}

func TestCalendarProvider_NoContent_ReturnsZeroHolidaySignal(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioNoContent)
	defer srv.Close()

	p := provider.NewCalendarProvider(&config.Config{CountryCode: "LK"})
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on 204 No Content: %v", err)
	}
	if len(signals) != 1 || signals[0].Value != 0.0 {
		t.Fatalf("expected a single public_holiday=0 signal on 204, got %+v", signals)
	}
}

func TestCalendarProvider_Malformed_ReturnsError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioMalformed)
	defer srv.Close()

	p := provider.NewCalendarProvider(&config.Config{CountryCode: "LK"})
	p.BaseURL = srv.URL

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatalf("expected a decode error against a truncated body, got nil")
	}
}

func TestCalendarProvider_ProviderError_ReturnsError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioProviderError)
	defer srv.Close()

	p := provider.NewCalendarProvider(&config.Config{CountryCode: "XX"})
	p.BaseURL = srv.URL

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatalf("expected an error against a non-200/204 status, got nil")
	}
}
