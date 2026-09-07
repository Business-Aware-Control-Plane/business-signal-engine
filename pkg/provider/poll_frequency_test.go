package provider_test

import (
	"testing"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
)

// TestPollFrequency_DefaultsUnchangedWhenUnset locks in the SIM-HB-01 §10
// guarantee: production cadence must not move unless a config override is
// explicitly set — an evaluation harness compressing timelines should never
// be able to accidentally leak into a default production run.
func TestPollFrequency_DefaultsUnchangedWhenUnset(t *testing.T) {
	cfg := &config.Config{}

	cases := []struct {
		name string
		freq time.Duration
		want time.Duration
	}{
		{"GoogleAnalytics", provider.NewGoogleAnalyticsProvider(cfg).PollFrequency(), 5 * time.Minute},
		{"MetaBusiness", provider.NewMetaBusinessProvider(cfg).PollFrequency(), 1 * time.Minute},
		{"Weather", provider.NewWeatherProvider(cfg).PollFrequency(), 15 * time.Minute},
		{"Calendar", provider.NewCalendarProvider(cfg).PollFrequency(), 30 * time.Minute},
		{"Prometheus", provider.NewPrometheusProvider(cfg).PollFrequency(), 1 * time.Minute},
		{"SocialMedia", provider.NewSocialMediaProvider(cfg).PollFrequency(), 2 * time.Minute},
		{"BusinessCalendar", provider.NewBusinessCalendarProvider(cfg).PollFrequency(), 30 * time.Minute},
		{"StripeReconciliation", provider.NewStripeReconciliationProvider(cfg).PollFrequency(), 1 * time.Hour},
	}
	for _, c := range cases {
		if c.freq != c.want {
			t.Errorf("%s: expected unchanged production default %s, got %s", c.name, c.want, c.freq)
		}
	}
}

// TestPollFrequency_OverrideWins verifies a set override always beats the
// production default, which is the whole point of the mechanism.
func TestPollFrequency_OverrideWins(t *testing.T) {
	cfg := &config.Config{
		GAPollInterval:                   5 * time.Second,
		MetaPollInterval:                 5 * time.Second,
		WeatherPollInterval:              5 * time.Second,
		CalendarPollInterval:             5 * time.Second,
		PrometheusPollInterval:           5 * time.Second,
		SocialMediaPollInterval:          5 * time.Second,
		BusinessCalendarPollInterval:     5 * time.Second,
		StripeReconciliationPollInterval: 5 * time.Second,
	}

	providers := []provider.SignalProvider{
		provider.NewGoogleAnalyticsProvider(cfg),
		provider.NewMetaBusinessProvider(cfg),
		provider.NewWeatherProvider(cfg),
		provider.NewCalendarProvider(cfg),
		provider.NewPrometheusProvider(cfg),
		provider.NewSocialMediaProvider(cfg),
		provider.NewBusinessCalendarProvider(cfg),
		provider.NewStripeReconciliationProvider(cfg),
	}
	for _, p := range providers {
		if got := p.PollFrequency(); got != 5*time.Second {
			t.Errorf("%s: expected overridden 5s poll interval, got %s", p.Name(), got)
		}
	}
}
