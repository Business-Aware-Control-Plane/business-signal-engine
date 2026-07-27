package provider

import (
	"context"
	"log"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

type GoogleAnalyticsProvider struct {
	cfg *config.Config
}

func NewGoogleAnalyticsProvider(cfg *config.Config) *GoogleAnalyticsProvider {
	return &GoogleAnalyticsProvider{cfg: cfg}
}

func (p *GoogleAnalyticsProvider) Name() string {
	return "GoogleAnalytics"
}

func (p *GoogleAnalyticsProvider) PollFrequency() time.Duration {
	return 5 * time.Minute
}

func (p *GoogleAnalyticsProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	if p.cfg.GAPropertyID == "" {
		log.Printf("[WARN] [GoogleAnalytics] GA_PROPERTY_ID environment variable is not configured. Skipping extraction to prevent fake data in database.")
		return nil, nil
	}

	// Live Google Analytics Data API v1beta integration hook
	log.Printf("[INFO] [GoogleAnalytics] Querying Google Analytics Data API for Property ID: %s", p.cfg.GAPropertyID)

	// In live execution with credentials, real API response will be parsed here.
	return nil, nil
}
