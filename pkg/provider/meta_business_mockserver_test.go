package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

func metaTestConfig() *config.Config {
	return &config.Config{MetaAccessToken: "test-token", MetaAdAccountID: "act_123"}
}

func TestMetaBusinessProvider_Success_ParsesRealSchema(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	p := provider.NewMetaBusinessProvider(metaTestConfig())
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 4 {
		t.Fatalf("expected 4 signals (spend, impressions, clicks, ctr), got %d", len(signals))
	}
	for _, s := range signals {
		if s.Type == "ad_spend_usd" && s.Value != 342.50 {
			t.Errorf("expected ad_spend_usd=342.50 from fixture, got %.2f", s.Value)
		}
	}
}

func TestMetaBusinessProvider_EmptyData_ReturnsNoSignalsNoError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioEmpty)
	defer srv.Close()

	p := provider.NewMetaBusinessProvider(metaTestConfig())
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on empty data: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals for an empty insights array, got %d", len(signals))
	}
}

func TestMetaBusinessProvider_APIErrorObject_ReturnsNoSignalsNoError(t *testing.T) {
	// Meta returns HTTP 200 with an {"error": {...}} body on auth failure —
	// a decode success at the transport level that still must not produce signals.
	srv := mockserver.NewServer(mockserver.ScenarioProviderError)
	defer srv.Close()

	p := provider.NewMetaBusinessProvider(metaTestConfig())
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("expected the API error object to be handled gracefully, got error: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals when Meta returns an error object, got %d", len(signals))
	}
}

func TestMetaBusinessProvider_Malformed_ReturnsError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioMalformed)
	defer srv.Close()

	p := provider.NewMetaBusinessProvider(metaTestConfig())
	p.BaseURL = srv.URL

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatalf("expected a decode error against a truncated body, got nil")
	}
}
