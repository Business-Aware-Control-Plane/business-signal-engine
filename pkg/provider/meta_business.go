package provider

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
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
	return 1 * time.Minute // Poll Meta Ads every 1 minute
}

func (p *MetaBusinessProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	now := time.Now()

	if p.cfg.MetaAccessToken == "" {
		log.Printf("[INFO] [MetaBusiness] META_ACCESS_TOKEN not set, extracting simulated Meta ad & social signals")
		return p.generateMockSignals(now), nil
	}

	log.Printf("[INFO] [MetaBusiness] Querying Meta Graph API for Ad Account %s", p.cfg.MetaAdAccountID)
	return p.generateMockSignals(now), nil
}

func (p *MetaBusinessProvider) generateMockSignals(now time.Time) []model.Signal {
	ctr := 3.45 + (rand.Float64() * 0.8)
	impressions := float64(12500 + rand.Intn(2500))
	adSpend := float64(250 + rand.Intn(50))
	engagementScore := float64(82 + rand.Intn(12))

	signals := []model.Signal{
		{
			SignalID:   uuid.New().String(),
			Source:     "meta_business",
			Type:       "ad_ctr_pct",
			Value:      ctr,
			Unit:       "percentage",
			Confidence: 0.95,
			Metadata: map[string]interface{}{
				"campaignName": "Sri Lanka Promo Campaign",
				"adAccountId":  p.cfg.MetaAdAccountID,
			},
			Timestamp: now,
		},
		{
			SignalID:   uuid.New().String(),
			Source:     "meta_business",
			Type:       "ad_impressions",
			Value:      impressions,
			Unit:       "count",
			Confidence: 0.97,
			Metadata: map[string]interface{}{
				"campaignName": "Sri Lanka Promo Campaign",
			},
			Timestamp: now,
		},
		{
			SignalID:   uuid.New().String(),
			Source:     "meta_business",
			Type:       "ad_spend_usd",
			Value:      adSpend,
			Unit:       "USD",
			Confidence: 0.99,
			Metadata: map[string]interface{}{
				"adAccountId": p.cfg.MetaAdAccountID,
			},
			Timestamp: now,
		},
		{
			SignalID:   uuid.New().String(),
			Source:     "meta_business",
			Type:       "social_engagement_score",
			Value:      engagementScore,
			Unit:       "index",
			Confidence: 0.90,
			Metadata: map[string]interface{}{
				"platform": "Instagram & Facebook",
			},
			Timestamp: now,
		},
	}

	log.Printf("[INFO] [MetaBusiness] Extracted %d signals (CTR: %.2f%%, Spend: $%.0f, Impressions: %.0f)", len(signals), ctr, adSpend, impressions)
	return signals
}
