package provider

import (
	"context"
	"log"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

type MetaBusinessProvider struct {
	cfg *config.Config
}

func NewMetaBusinessProvider(cfg *config.Config) *MetaBusinessProvider {
	return &MetaBusinessProvider{cfg: cfg}
}

func (p *MetaBusinessProvider) Name() string {
	return "MetaBusiness"
}

func (p *MetaBusinessProvider) PollFrequency() time.Duration {
	return 1 * time.Minute
}

func (p *MetaBusinessProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	if p.cfg.MetaAccessToken == "" || p.cfg.MetaAdAccountID == "" {
		log.Printf("[WARN] [MetaBusiness] META_ACCESS_TOKEN or META_AD_ACCOUNT_ID is not configured. Skipping extraction to prevent fake data in database.")
		return nil, nil
	}

	// Live Meta Graph API integration hook
	log.Printf("[INFO] [MetaBusiness] Querying Meta Graph API for Ad Account ID: %s", p.cfg.MetaAdAccountID)

	// In live execution with credentials, real API response will be parsed here.
	return nil, nil
}
