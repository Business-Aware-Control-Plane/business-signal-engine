package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
)

type MetaBusinessProvider struct {
	cfg        *config.Config
	httpClient *http.Client
}

type metaInsightsResponse struct {
	Data []struct {
		Spend       string `json:"spend"`
		Impressions string `json:"impressions"`
		Clicks      string `json:"clicks"`
		CTR         string `json:"ctr"`
		CPC         string `json:"cpc"`
		DateStart   string `json:"date_start"`
		DateStop    string `json:"date_stop"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func NewMetaBusinessProvider(cfg *config.Config) *MetaBusinessProvider {
	return &MetaBusinessProvider{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
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

	adAccountID := p.cfg.MetaAdAccountID
	if !strings.HasPrefix(adAccountID, "act_") {
		adAccountID = "act_" + adAccountID
	}

	log.Printf("[INFO] [MetaBusiness] Querying Meta Graph API for Ad Account ID: %s", adAccountID)

	url := fmt.Sprintf(
		"https://graph.facebook.com/v19.0/%s/insights?fields=spend,impressions,clicks,ctr,cpc&date_preset=today&access_token=%s",
		adAccountID,
		p.cfg.MetaAccessToken,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Meta Graph API request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("[WARN] [MetaBusiness] Meta Graph API request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	var insights metaInsightsResponse
	if err := json.NewDecoder(resp.Body).Decode(&insights); err != nil {
		return nil, fmt.Errorf("failed to decode Meta Graph API response: %w", err)
	}

	if insights.Error != nil {
		log.Printf("[WARN] [MetaBusiness] Meta Graph API returned error (code %d): %s", insights.Error.Code, insights.Error.Message)
		return nil, nil
	}

	now := time.Now()
	var signals []model.Signal

	if len(insights.Data) > 0 {
		d := insights.Data[0]
		spend, _ := strconv.ParseFloat(d.Spend, 64)
		impressions, _ := strconv.ParseFloat(d.Impressions, 64)
		clicks, _ := strconv.ParseFloat(d.Clicks, 64)
		ctr, _ := strconv.ParseFloat(d.CTR, 64)

		signals = append(signals,
			model.Signal{
				SignalID:   uuid.New().String(),
				Source:     "meta_business",
				Type:       "ad_spend_usd",
				Value:      spend,
				Unit:       "USD",
				Confidence: 0.99,
				Metadata: map[string]interface{}{
					"adAccountId": adAccountID,
				},
				Timestamp: now,
			},
			model.Signal{
				SignalID:   uuid.New().String(),
				Source:     "meta_business",
				Type:       "ad_impressions",
				Value:      impressions,
				Unit:       "count",
				Confidence: 0.98,
				Metadata: map[string]interface{}{
					"adAccountId": adAccountID,
				},
				Timestamp: now,
			},
			model.Signal{
				SignalID:   uuid.New().String(),
				Source:     "meta_business",
				Type:       "ad_clicks",
				Value:      clicks,
				Unit:       "count",
				Confidence: 0.98,
				Metadata: map[string]interface{}{
					"adAccountId": adAccountID,
				},
				Timestamp: now,
			},
			model.Signal{
				SignalID:   uuid.New().String(),
				Source:     "meta_business",
				Type:       "ad_ctr_pct",
				Value:      ctr,
				Unit:       "percentage",
				Confidence: 0.95,
				Metadata: map[string]interface{}{
					"adAccountId": adAccountID,
				},
				Timestamp: now,
			},
		)

		log.Printf("[INFO] [MetaBusiness] Successfully extracted %d signals (Spend: $%.2f, Impressions: %.0f, Clicks: %.0f, CTR: %.2f%%)", len(signals), spend, impressions, clicks, ctr)
	} else {
		log.Printf("[INFO] [MetaBusiness] No ad activity returned for today for Ad Account %s", adAccountID)
	}

	return signals, nil
}
