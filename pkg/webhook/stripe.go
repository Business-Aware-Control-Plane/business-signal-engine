package webhook

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
)

// signatureTolerance guards against a captured webhook being replayed long
// after the fact being accepted as if it just happened.
const signatureTolerance = 5 * time.Minute

// maxBodyBytes bounds how much of a webhook body is read before giving up —
// a real Stripe event is a few KB; anything wildly larger is not a delivery
// worth trusting.
const maxBodyBytes = 64 * 1024

// stripeEvent is the minimal slice of Stripe's real event envelope this
// ingestor needs: id + type for idempotency and classification, and the
// charge/payment-intent amount as the actual demand signal. Stripe's full
// event payload carries far more fields; everything else is intentionally
// left unmodeled rather than guessed at.
type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object struct {
			Amount   int64  `json:"amount"` // minor units (cents)
			Currency string `json:"currency"`
			Status   string `json:"status"`
		} `json:"object"`
	} `json:"data"`
}

// Sink is the minimal interface this handler needs from the collector —
// narrowed on purpose so pkg/webhook never has to import pkg/pipeline.
type Sink interface {
	Ingest(model.Signal)
}

// NewStripeHandler returns an http.HandlerFunc implementing the two
// non-negotiable pieces production webhook architectures converge on: HMAC
// signature verification on the raw body, and idempotency on the event ID,
// so a retried delivery (Stripe retries anything that isn't a 2xx) is
// acknowledged without the underlying signal being double-counted.
func NewStripeHandler(webhookSecret string, sink Sink, idempo *IdempotencyStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		if err != nil {
			http.Error(w, "unable to read body", http.StatusBadRequest)
			return
		}

		if webhookSecret == "" {
			log.Printf("[WARN] [Webhook/Stripe] STRIPE_WEBHOOK_SECRET not configured; rejecting webhook rather than processing unverified data")
			http.Error(w, "webhook not configured", http.StatusServiceUnavailable)
			return
		}

		if err := VerifyStripeSignature(webhookSecret, body, r.Header.Get("Stripe-Signature"), signatureTolerance); err != nil {
			log.Printf("[WARN] [Webhook/Stripe] Signature verification failed: %v", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		var evt stripeEvent
		if err := json.Unmarshal(body, &evt); err != nil {
			log.Printf("[WARN] [Webhook/Stripe] Failed to parse event body: %v", err)
			http.Error(w, "malformed event", http.StatusBadRequest)
			return
		}
		if evt.ID == "" {
			http.Error(w, "missing event id", http.StatusBadRequest)
			return
		}

		if !idempo.MarkIfNew(evt.ID) {
			log.Printf("[INFO] [Webhook/Stripe] Duplicate delivery of %s ignored (already processed)", evt.ID)
			w.WriteHeader(http.StatusOK)
			return
		}

		if signal, ok := mapStripeEventToSignal(evt); ok {
			sink.Ingest(signal)
			log.Printf("[INFO] [Webhook/Stripe] Ingested %s (%s %.2f)", evt.Type, signal.Unit, signal.Value)
		} else {
			log.Printf("[DEBUG] [Webhook/Stripe] Acknowledged event type '%s' — not a modeled demand signal", evt.Type)
		}
		w.WriteHeader(http.StatusOK)
	}
}

// mapStripeEventToSignal converts a Stripe event into a business signal.
// Not every Stripe event type is a demand signal (e.g. customer.updated) —
// those are still acknowledged with 200 so Stripe doesn't retry them
// forever, but produce no signal.
func mapStripeEventToSignal(evt stripeEvent) (model.Signal, bool) {
	switch evt.Type {
	case "charge.succeeded", "payment_intent.succeeded":
		return model.Signal{
			SignalID:   uuid.New().String(),
			Source:     "stripe",
			Type:       "payment_succeeded",
			Value:      float64(evt.Data.Object.Amount) / 100.0,
			Unit:       evt.Data.Object.Currency,
			Confidence: 0.99,
			Metadata: map[string]interface{}{
				"stripeEventId":   evt.ID,
				"stripeEventType": evt.Type,
				"status":          evt.Data.Object.Status,
			},
			Timestamp: time.Now(),
		}, true
	default:
		return model.Signal{}, false
	}
}
