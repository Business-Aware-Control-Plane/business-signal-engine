package webhook_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/webhook"
)

// fakeSink records every signal handed to Ingest, standing in for Collector
// without pulling pkg/pipeline into this package's tests.
type fakeSink struct {
	mu       sync.Mutex
	ingested []model.Signal
}

func (f *fakeSink) Ingest(s model.Signal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ingested = append(f.ingested, s)
}

func (f *fakeSink) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ingested)
}

const stripeSecret = "whsec_test_secret"

func postStripeWebhook(t *testing.T, handler http.HandlerFunc, body string, signWith string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	if signWith != "" {
		req.Header.Set("Stripe-Signature", signStripe(signWith, time.Now().Unix(), []byte(body)))
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestStripeHandler_ValidChargeSucceeded_IngestsSignal(t *testing.T) {
	sink := &fakeSink{}
	idempo := webhook.NewIdempotencyStore(time.Hour)
	handler := webhook.NewStripeHandler(stripeSecret, sink, idempo)

	body := `{"id":"evt_1","type":"charge.succeeded","data":{"object":{"amount":5000,"currency":"usd","status":"succeeded"}}}`
	rec := postStripeWebhook(t, handler, body, stripeSecret)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if sink.count() != 1 {
		t.Fatalf("expected 1 ingested signal, got %d", sink.count())
	}
	if sink.ingested[0].Value != 50.0 || sink.ingested[0].Unit != "usd" {
		t.Fatalf("expected amount converted from cents (5000 -> 50.0 usd), got %+v", sink.ingested[0])
	}
}

func TestStripeHandler_InvalidSignature_Rejected(t *testing.T) {
	sink := &fakeSink{}
	idempo := webhook.NewIdempotencyStore(time.Hour)
	handler := webhook.NewStripeHandler(stripeSecret, sink, idempo)

	body := `{"id":"evt_1","type":"charge.succeeded","data":{"object":{"amount":5000,"currency":"usd"}}}`
	rec := postStripeWebhook(t, handler, body, "wrong_secret")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an invalid signature, got %d", rec.Code)
	}
	if sink.count() != 0 {
		t.Fatalf("expected no signal ingested for an unverified request, got %d", sink.count())
	}
}

func TestStripeHandler_DuplicateDelivery_AcknowledgedNotIngestedTwice(t *testing.T) {
	sink := &fakeSink{}
	idempo := webhook.NewIdempotencyStore(time.Hour)
	handler := webhook.NewStripeHandler(stripeSecret, sink, idempo)

	body := `{"id":"evt_dup","type":"charge.succeeded","data":{"object":{"amount":1000,"currency":"usd"}}}`

	first := postStripeWebhook(t, handler, body, stripeSecret)
	second := postStripeWebhook(t, handler, body, stripeSecret)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("expected both deliveries to be acknowledged with 200 (first=%d second=%d)", first.Code, second.Code)
	}
	if sink.count() != 1 {
		t.Fatalf("expected exactly 1 ingested signal across 2 identical deliveries, got %d", sink.count())
	}
}

func TestStripeHandler_UnmodeledEventType_AcknowledgedButNotIngested(t *testing.T) {
	sink := &fakeSink{}
	idempo := webhook.NewIdempotencyStore(time.Hour)
	handler := webhook.NewStripeHandler(stripeSecret, sink, idempo)

	body := `{"id":"evt_2","type":"customer.updated","data":{"object":{}}}`
	rec := postStripeWebhook(t, handler, body, stripeSecret)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected an unmodeled but validly signed event to still be acknowledged with 200, got %d", rec.Code)
	}
	if sink.count() != 0 {
		t.Fatalf("expected no signal for an event type outside the demand-signal mapping, got %d", sink.count())
	}
}

func TestStripeHandler_MissingSecret_RejectsRatherThanProcessingUnverified(t *testing.T) {
	sink := &fakeSink{}
	idempo := webhook.NewIdempotencyStore(time.Hour)
	handler := webhook.NewStripeHandler("", sink, idempo) // STRIPE_WEBHOOK_SECRET unset

	body := `{"id":"evt_3","type":"charge.succeeded","data":{"object":{"amount":100,"currency":"usd"}}}`
	rec := postStripeWebhook(t, handler, body, "")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when the webhook secret is unconfigured, got %d", rec.Code)
	}
	if sink.count() != 0 {
		t.Fatalf("expected no signal ingested without a configured secret, got %d", sink.count())
	}
}
