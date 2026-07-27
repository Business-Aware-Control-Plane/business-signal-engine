package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
)

func TestSimulatorStreamExtraction(t *testing.T) {
	cfg := &config.Config{
		CountryCode: "LK",
		Latitude:    6.9271,
		Longitude:   79.8612,
	}

	simProvider := provider.NewSimulatorProvider(cfg)
	signals, err := simProvider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("SimulatorStream Fetch failed: %v", err)
	}

	if len(signals) == 0 {
		t.Errorf("SimulatorStream returned 0 signals")
	}

	for _, s := range signals {
		if s.Source == "" {
			t.Errorf("Simulator signal has empty Source")
		}
		if s.Type == "" {
			t.Errorf("Simulator signal has empty Type")
		}
		if s.Confidence < 0.0 || s.Confidence > 1.0 {
			t.Errorf("Simulator signal has invalid confidence score: %f", s.Confidence)
		}
	}
}

func TestUnsetCredentialsHandling(t *testing.T) {
	cfg := &config.Config{}
	ctx := context.Background()

	ga := provider.NewGoogleAnalyticsProvider(cfg)
	signals, err := ga.Fetch(ctx)
	if err != nil {
		t.Errorf("GoogleAnalytics should not return error on unset credentials: %v", err)
	}
	if len(signals) != 0 {
		t.Errorf("GoogleAnalytics should return 0 signals when credentials are unset to avoid fake data, got %d", len(signals))
	}

	meta := provider.NewMetaBusinessProvider(cfg)
	signals, err = meta.Fetch(ctx)
	if err != nil {
		t.Errorf("MetaBusiness should not return error on unset credentials: %v", err)
	}
	if len(signals) != 0 {
		t.Errorf("MetaBusiness should return 0 signals when credentials are unset to avoid fake data, got %d", len(signals))
	}
}
