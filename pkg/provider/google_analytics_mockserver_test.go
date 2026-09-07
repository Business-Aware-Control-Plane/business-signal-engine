package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

func TestGoogleAnalyticsProvider_Success_ParsesRealSchema(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	p := provider.NewGoogleAnalyticsProvider(&config.Config{
		GAPropertyID: "123456789",
		GAEndpoint:   srv.URL,
	})

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4 realtime (active_users, screen_page_views, event_count, key_events)
	// + 4 core (engagement_rate, user_engagement_duration, bounce_rate, conversions)
	if len(signals) != 8 {
		t.Fatalf("expected 8 GA4 signals from the mock realtime+core reports, got %d: %+v", len(signals), signals)
	}

	found := map[string]float64{}
	for _, s := range signals {
		found[s.Type] = s.Value
	}
	if found["active_users"] != 482 {
		t.Errorf("expected active_users=482 from fixture, got %v", found["active_users"])
	}
	if found["conversions"] != 9 {
		t.Errorf("expected conversions=9 from fixture, got %v", found["conversions"])
	}
}
