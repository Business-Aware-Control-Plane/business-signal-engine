package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

// Prometheus was already configurable via cfg.PrometheusURL before this
// review — no BaseURL field needed, just pointed at the fixture server.

func TestPrometheusProvider_Success_ParsesRealSchema(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	p := provider.NewPrometheusProvider(&config.Config{PrometheusURL: srv.URL})
	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 4 {
		t.Fatalf("expected 4 infrastructure metrics, got %d", len(signals))
	}
	for _, s := range signals {
		if s.Value != 23.7 {
			t.Errorf("expected fixture value 23.7 for metric %s, got %.2f", s.Type, s.Value)
		}
	}
}

func TestPrometheusProvider_Malformed_DegradesToZeroRatherThanErroring(t *testing.T) {
	// Current provider behaviour: a query-level decode failure logs a warning
	// and reports 0.0 rather than propagating an error from Fetch. This test
	// pins that behaviour down explicitly rather than leaving it unverified.
	srv := mockserver.NewServer(mockserver.ScenarioMalformed)
	defer srv.Close()

	p := provider.NewPrometheusProvider(&config.Config{PrometheusURL: srv.URL})
	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch is expected to swallow per-query errors, got: %v", err)
	}
	for _, s := range signals {
		if s.Value != 0.0 {
			t.Errorf("expected malformed response to degrade to 0.0 for %s, got %.2f", s.Type, s.Value)
		}
	}
}
