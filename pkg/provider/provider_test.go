package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
)

func TestProvidersSignalExtraction(t *testing.T) {
	cfg := &config.Config{
		CountryCode: "LK",
		Latitude:    6.9271,
		Longitude:   79.8612,
	}

	ctx := context.Background()

	providers := []provider.SignalProvider{
		provider.NewGoogleAnalyticsProvider(cfg),
		provider.NewMetaBusinessProvider(cfg),
		provider.NewWeatherProvider(cfg),
		provider.NewCalendarProvider(cfg),
	}

	for _, p := range providers {
		t.Run(p.Name(), func(t *testing.T) {
			signals, err := p.Fetch(ctx)
			if err != nil {
				t.Fatalf("Provider %s Fetch failed: %v", p.Name(), err)
			}

			if len(signals) == 0 {
				t.Errorf("Provider %s returned 0 signals", p.Name())
			}

			for _, s := range signals {
				if s.Source == "" {
					t.Errorf("Signal from %s has empty Source", p.Name())
				}
				if s.Type == "" {
					t.Errorf("Signal from %s has empty Type", p.Name())
				}
				if s.Confidence < 0.0 || s.Confidence > 1.0 {
					t.Errorf("Signal from %s has invalid confidence score: %f", p.Name(), s.Confidence)
				}
				if s.Timestamp.IsZero() {
					t.Errorf("Signal from %s has zero timestamp", p.Name())
				}
			}
		})
	}
}
