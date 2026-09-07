package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

func TestWeatherProvider_Success_ParsesRealSchema(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	p := provider.NewWeatherProvider(&config.Config{CountryCode: "LK", Latitude: 6.9271, Longitude: 79.8612})
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 3 {
		t.Fatalf("expected 3 signals (temperature, rain_mm, humidity_pct), got %d", len(signals))
	}

	var rain *float64
	for _, s := range signals {
		if s.Type == "rain_mm" {
			v := s.Value
			rain = &v
			if heavy, _ := s.Metadata["isHeavyRain"].(bool); !heavy {
				t.Errorf("expected isHeavyRain=true for 12.4mm, got metadata=%v", s.Metadata)
			}
		}
	}
	if rain == nil || *rain != 12.4 {
		t.Fatalf("expected rain_mm=12.4 from fixture, got %v", rain)
	}
}

func TestWeatherProvider_Malformed_ReturnsError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioMalformed)
	defer srv.Close()

	p := provider.NewWeatherProvider(&config.Config{})
	p.BaseURL = srv.URL

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatalf("expected a decode error against a truncated body, got nil")
	}
}

func TestWeatherProvider_ProviderError_ReturnsError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioProviderError)
	defer srv.Close()

	p := provider.NewWeatherProvider(&config.Config{})
	p.BaseURL = srv.URL

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatalf("expected an error against a non-200 status, got nil")
	}
}
