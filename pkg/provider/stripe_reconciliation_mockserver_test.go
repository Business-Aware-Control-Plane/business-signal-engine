package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

func TestStripeReconciliationProvider_Success_ExcludesFailedCharges(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	p := provider.NewStripeReconciliationProvider(&config.Config{StripeSecretKey: "sk_test"})
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 rolled-up reconciliation signal, got %d", len(signals))
	}
	// Fixture: 2500 + 1500 succeeded (900 failed excluded) = 4000 cents = 40.00
	if signals[0].Value != 40.0 {
		t.Fatalf("expected reconciled total 40.00 (excluding the failed charge), got %.2f", signals[0].Value)
	}
	if count, _ := signals[0].Metadata["chargeCount"].(int); count != 2 {
		t.Fatalf("expected chargeCount=2, got %v", signals[0].Metadata["chargeCount"])
	}
}

func TestStripeReconciliationProvider_Empty_ReturnsNoSignal(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioEmpty)
	defer srv.Close()

	p := provider.NewStripeReconciliationProvider(&config.Config{StripeSecretKey: "sk_test"})
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals when no charges occurred, got %d", len(signals))
	}
}

func TestStripeReconciliationProvider_UnsetSecretKey_SkipsWithoutError(t *testing.T) {
	p := provider.NewStripeReconciliationProvider(&config.Config{})

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("expected no error when STRIPE_SECRET_KEY is unset, got: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals when unconfigured, got %d", len(signals))
	}
}

func TestStripeReconciliationProvider_Malformed_ReturnsError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioMalformed)
	defer srv.Close()

	p := provider.NewStripeReconciliationProvider(&config.Config{StripeSecretKey: "sk_test"})
	p.BaseURL = srv.URL

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatalf("expected a decode error against a truncated body, got nil")
	}
}
