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

// SocialMediaProvider reports organic engagement on the business's own Page —
// closing BSAL-HB-01 §04's gap (Marketing/Social event categories previously
// had no discrete source; only ad spend/CTR existed). It reuses Meta's
// Page Insights edge, distinct from the paid-ads Marketing API endpoint
// MetaBusinessProvider already covers.
type SocialMediaProvider struct {
	cfg        *config.Config
	httpClient *http.Client
	BaseURL    string
}

// metaInsightsMetric mirrors the real shape of Meta's Page Insights API:
// {"data":[{"name":"page_post_engagements","period":"day","values":[{"value":342,"end_time":"..."}]}]}
type metaPageInsightsResponse struct {
	Data []struct {
		Name   string `json:"name"`
		Period string `json:"period"`
		Values []struct {
			Value   float64 `json:"value"`
			EndTime string  `json:"end_time"`
		} `json:"values"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func NewSocialMediaProvider(cfg *config.Config) *SocialMediaProvider {
	baseURL := cfg.MetaGraphBaseURL
	if baseURL == "" {
		baseURL = "https://graph.facebook.com/v19.0"
	}
	return &SocialMediaProvider{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		BaseURL:    baseURL,
	}
}

func (p *SocialMediaProvider) Name() string { return "SocialMedia" }

// Poll more frequently than paid-ads spend, since organic virality (the
// perfume-store Instagram Reel scenario the whole project is framed around)
// can spike within minutes, not the hour-scale cadence ad spend changes at.
func (p *SocialMediaProvider) PollFrequency() time.Duration {
	return pollFrequencyOr(p.cfg.SocialMediaPollInterval, 2*time.Minute)
}

func (p *SocialMediaProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	if p.cfg.MetaAccessToken == "" || p.cfg.MetaPageID == "" {
		log.Printf("[WARN] [SocialMedia] META_ACCESS_TOKEN or META_PAGE_ID is not configured. Skipping extraction to prevent fake data in database.")
		return nil, nil
	}

	url := fmt.Sprintf(
		"%s/%s/insights?metric=page_post_engagements,page_engaged_users,page_impressions&period=day&access_token=%s",
		p.BaseURL,
		p.cfg.MetaPageID,
		p.cfg.MetaAccessToken,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Meta Page Insights request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("[WARN] [SocialMedia] Meta Page Insights request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	var insights metaPageInsightsResponse
	if err := json.NewDecoder(resp.Body).Decode(&insights); err != nil {
		return nil, fmt.Errorf("failed to decode Meta Page Insights response: %w", err)
	}

	if insights.Error != nil {
		log.Printf("[WARN] [SocialMedia] Meta Page Insights API returned error (code %d): %s", insights.Error.Code, insights.Error.Message)
		return nil, nil
	}

	now := time.Now()
	var signals []model.Signal
	for _, metric := range insights.Data {
		if len(metric.Values) == 0 {
			continue
		}
		signals = append(signals, model.Signal{
			SignalID:   uuid.New().String(),
			Source:     "social_media",
			Type:       metric.Name,
			Value:      metric.Values[len(metric.Values)-1].Value, // most recent reporting period
			Unit:       "count",
			Confidence: 0.9,
			Metadata: map[string]interface{}{
				"pageId": p.cfg.MetaPageID,
				"period": metric.Period,
			},
			Timestamp: now,
		})
	}

	log.Printf("[INFO] [SocialMedia] Extracted %d organic engagement signals for Page %s", len(signals), p.cfg.MetaPageID)
	return signals, nil
}
