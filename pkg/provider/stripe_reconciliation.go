package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
)

// StripeReconciliationProvider is the low-frequency polling half of the
// two-path webhook architecture described in BSAL-HB-01 §05: the webhook
// handler (pkg/webhook) is the real-time path, and this is the scheduled
// fallback that catches a delivery the webhook missed (an outage, a dropped
// TCP connection, a misconfigured endpoint) — not a replacement for it.
type StripeReconciliationProvider struct {
	cfg        *config.Config
	httpClient *http.Client
	BaseURL    string

	lastPoll time.Time
}

type stripeChargeListResponse struct {
	Data []struct {
		ID       string `json:"id"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
		Status   string `json:"status"`
		Created  int64  `json:"created"`
	} `json:"data"`
}

func NewStripeReconciliationProvider(cfg *config.Config) *StripeReconciliationProvider {
	baseURL := cfg.StripeBaseURL
	if baseURL == "" {
		baseURL = "https://api.stripe.com/v1"
	}
	return &StripeReconciliationProvider{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		BaseURL:    baseURL,
		lastPoll:   time.Now().Add(-1 * time.Hour), // first poll covers the last hour
	}
}

func (p *StripeReconciliationProvider) Name() string { return "StripeReconciliation" }

// Deliberately hourly — this exists to catch what the webhook missed, not
// to be the primary path. A tighter interval would defeat the point of
// having a push path at all.
func (p *StripeReconciliationProvider) PollFrequency() time.Duration {
	return pollFrequencyOr(p.cfg.StripeReconciliationPollInterval, 1*time.Hour)
}

func (p *StripeReconciliationProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	if p.cfg.StripeSecretKey == "" {
		log.Printf("[WARN] [StripeReconciliation] STRIPE_SECRET_KEY is not configured. Skipping extraction to prevent fake data in database.")
		return nil, nil
	}

	windowStart := p.lastPoll
	now := time.Now()

	url := fmt.Sprintf("%s/charges?created[gte]=%d&limit=100", p.BaseURL, windowStart.Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe reconciliation request: %w", err)
	}
	req.SetBasicAuth(p.cfg.StripeSecretKey, "") // Stripe API convention: secret key as username, empty password

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("[WARN] [StripeReconciliation] Stripe API request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe API returned status code %d", resp.StatusCode)
	}

	var charges stripeChargeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&charges); err != nil {
		return nil, fmt.Errorf("failed to decode Stripe charges response: %w", err)
	}

	var totalMinorUnits int64
	succeededCount := 0
	for _, c := range charges.Data {
		if c.Status == "succeeded" {
			totalMinorUnits += c.Amount
			succeededCount++
		}
	}
	p.lastPoll = now

	if succeededCount == 0 {
		log.Printf("[INFO] [StripeReconciliation] No succeeded charges since %s", windowStart.Format(time.RFC3339))
		return nil, nil
	}

	signal := model.Signal{
		SignalID:   uuid.New().String(),
		Source:     "stripe",
		Type:       "payment_volume_reconciled",
		Value:      float64(totalMinorUnits) / 100.0,
		Unit:       "aggregate",
		Confidence: 0.9, // a rolled-up reconciliation check, not a direct real-time reading
		Metadata: map[string]interface{}{
			"chargeCount":    succeededCount,
			"windowStart":    windowStart.Format(time.RFC3339),
			"windowEnd":      now.Format(time.RFC3339),
			"reconciliation": true,
		},
		Timestamp: now,
	}

	log.Printf("[INFO] [StripeReconciliation] Reconciled %d succeeded charges (%.2f total) since %s", succeededCount, signal.Value, windowStart.Format(time.RFC3339))
	return []model.Signal{signal}, nil
}
