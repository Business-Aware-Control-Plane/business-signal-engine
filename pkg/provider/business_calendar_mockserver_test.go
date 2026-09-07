package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

func TestBusinessCalendarProvider_Success_SeparatesActiveFromUpcoming(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	p := provider.NewBusinessCalendarProvider(&config.Config{
		BusinessCalendarID:  "marketing@example.com",
		CalendarAPIEndpoint: srv.URL,
	})

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 2 {
		t.Fatalf("expected 1 active + 1 upcoming signal from the fixture, got %d: %+v", len(signals), signals)
	}

	var sawActiveCampaign, sawUpcomingLaunch bool
	for _, s := range signals {
		if s.Type == "campaign_window" && s.Value == 1 {
			sawActiveCampaign = true
		}
		if s.Type == "product_launch_upcoming" && s.Value == 1 {
			sawUpcomingLaunch = true
			if _, ok := s.Metadata["startsInHours"]; !ok {
				t.Errorf("expected startsInHours metadata on an upcoming event, got %v", s.Metadata)
			}
		}
	}
	if !sawActiveCampaign {
		t.Errorf("expected the currently-running 'Flash Sale Campaign' to classify as an active campaign_window")
	}
	if !sawUpcomingLaunch {
		t.Errorf("expected the 48h-out 'Aqua Line Product Launch' to classify as product_launch_upcoming")
	}
}

func TestBusinessCalendarProvider_Empty_ReturnsNoSignals(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioEmpty)
	defer srv.Close()

	p := provider.NewBusinessCalendarProvider(&config.Config{
		BusinessCalendarID:  "marketing@example.com",
		CalendarAPIEndpoint: srv.URL,
	})

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals for an empty calendar, got %d", len(signals))
	}
}

func TestBusinessCalendarProvider_Malformed_ReturnsError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioMalformed)
	defer srv.Close()

	p := provider.NewBusinessCalendarProvider(&config.Config{
		BusinessCalendarID:  "marketing@example.com",
		CalendarAPIEndpoint: srv.URL,
	})

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatalf("expected a decode error against a truncated body, got nil")
	}
}

func TestBusinessCalendarProvider_UnsetCalendarID_SkipsWithoutError(t *testing.T) {
	p := provider.NewBusinessCalendarProvider(&config.Config{})

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("expected no error when BUSINESS_CALENDAR_ID is unset, got: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals when unconfigured, got %d", len(signals))
	}
}
