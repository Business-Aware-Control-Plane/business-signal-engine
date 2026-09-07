package provider_test

import (
	"context"
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

func socialTestConfig() *config.Config {
	return &config.Config{MetaAccessToken: "test-token", MetaPageID: "123456789012345"}
}

func TestSocialMediaProvider_Success_ParsesPageInsightsSchema(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	p := provider.NewSocialMediaProvider(socialTestConfig())
	p.BaseURL = srv.URL

	signals, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 3 {
		t.Fatalf("expected 3 organic engagement signals, got %d", len(signals))
	}
	for _, s := range signals {
		if s.Source != "social_media" {
			t.Errorf("expected source=social_media, got %s", s.Source)
		}
		if s.Type == "page_post_engagements" && s.Value != 342 {
			t.Errorf("expected page_post_engagements=342 from fixture, got %.0f", s.Value)
		}
	}
}

// This is the regression test for the real bug caught while building the
// fixture server: MetaBusinessProvider (ads) and SocialMediaProvider (page)
// both call a path ending in "/insights" but expect different JSON shapes.
// If the mock server ever routes them to the same handler again, this fails
// with a decode error instead of silently parsing the wrong shape.
func TestSocialMediaProvider_DoesNotCollideWithAdsInsightsShape(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioSuccess)
	defer srv.Close()

	social := provider.NewSocialMediaProvider(socialTestConfig())
	social.BaseURL = srv.URL
	ads := provider.NewMetaBusinessProvider(metaTestConfig())
	ads.BaseURL = srv.URL

	socialSignals, err := social.Fetch(context.Background())
	if err != nil {
		t.Fatalf("social fetch failed: %v", err)
	}
	adsSignals, err := ads.Fetch(context.Background())
	if err != nil {
		t.Fatalf("ads fetch failed: %v", err)
	}
	if len(socialSignals) != 3 || len(adsSignals) != 4 {
		t.Fatalf("expected 3 social + 4 ads signals from their respective shapes, got social=%d ads=%d", len(socialSignals), len(adsSignals))
	}
}

func TestSocialMediaProvider_Malformed_ReturnsError(t *testing.T) {
	srv := mockserver.NewServer(mockserver.ScenarioMalformed)
	defer srv.Close()

	p := provider.NewSocialMediaProvider(socialTestConfig())
	p.BaseURL = srv.URL

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatalf("expected a decode error against a truncated body, got nil")
	}
}
