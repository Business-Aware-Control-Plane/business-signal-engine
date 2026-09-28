package significance_test

import (
	"testing"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/memory"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/processor"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/significance"
)

func TestEvaluate_NoBaselinesNoScheduledEvent_NotSignificant(t *testing.T) {
	signals := []model.Signal{{Source: "google_analytics", Type: "active_users", Value: 120}}
	baselines := map[string]memory.BaselineComparison{
		"google_analytics:active_users": {SeasonalZScore: 0.4},
	}
	res := significance.Evaluate(signals, baselines, processor.GuardrailResult{})

	if res.IsSignificant {
		t.Fatalf("expected not significant, got IsSignificant=true (score=%.2f)", res.Score)
	}
}

func TestEvaluate_HighSeasonalZScore_IsSignificant(t *testing.T) {
	signals := []model.Signal{{Source: "weather", Type: "rain_mm", Value: 18}}
	baselines := map[string]memory.BaselineComparison{
		"weather:rain_mm": {SeasonalZScore: 3.1},
	}
	res := significance.Evaluate(signals, baselines, processor.GuardrailResult{})

	if !res.IsSignificant {
		t.Fatalf("expected significant for z=3.1, got false")
	}
	if len(res.TriggeredRules) != 1 || res.TriggeredRules[0] != "seasonal_z:weather:rain_mm=3.10" {
		t.Fatalf("unexpected triggered rules: %v", res.TriggeredRules)
	}
}

func TestEvaluate_ScheduledEvent_SignificantRegardlessOfZScore(t *testing.T) {
	signals := []model.Signal{{Source: "calendar", Type: "public_holiday", Value: 1}}
	res := significance.Evaluate(signals, map[string]memory.BaselineComparison{}, processor.GuardrailResult{})

	if !res.IsSignificant {
		t.Fatalf("expected a scheduled event (public_holiday=1) to be significant on its own")
	}
}

func TestEvaluate_ScheduledEventFlagZero_NotSignificantOnItsOwn(t *testing.T) {
	signals := []model.Signal{{Source: "calendar", Type: "public_holiday", Value: 0}}
	res := significance.Evaluate(signals, map[string]memory.BaselineComparison{}, processor.GuardrailResult{})

	if res.IsSignificant {
		t.Fatalf("public_holiday=0 should not itself trigger significance")
	}
}

// ---- Corroboration: a reach-style social spike needs engaged_users to
// move too, otherwise it's exactly the bot/inorganic-amplification pattern
// Scenario 6 (false-positive-social-spike) is meant to test. See SIM-HB-01
// §08 for why "did real traffic eventually follow" can never be the basis
// for this rule — only what's observable in the current window can be.

func TestEvaluate_ReachSpikeWithoutEngagedUsersCorroboration_NotSignificant(t *testing.T) {
	signals := []model.Signal{{Source: "social_media", Type: "page_impressions", Value: 9600}}
	baselines := map[string]memory.BaselineComparison{
		"social_media:page_impressions":   {SeasonalZScore: 6.0}, // reach spikes hard
		"social_media:page_engaged_users": {SeasonalZScore: 0.2}, // but almost nobody real engaged
	}
	res := significance.Evaluate(signals, baselines, processor.GuardrailResult{})

	if res.IsSignificant {
		t.Fatalf("expected an uncorroborated reach spike (impressions moved, engaged_users didn't) to stay insignificant, got true (score=%.2f, rules=%v)", res.Score, res.TriggeredRules)
	}
}

func TestEvaluate_ReachSpikeWithEngagedUsersCorroboration_IsSignificant(t *testing.T) {
	signals := []model.Signal{{Source: "social_media", Type: "page_impressions", Value: 9600}}
	baselines := map[string]memory.BaselineComparison{
		"social_media:page_impressions":   {SeasonalZScore: 6.0}, // reach spikes
		"social_media:page_engaged_users": {SeasonalZScore: 5.5}, // and real people actually engaged too
	}
	res := significance.Evaluate(signals, baselines, processor.GuardrailResult{})

	if !res.IsSignificant {
		t.Fatalf("expected a corroborated reach spike (both impressions and engaged_users moved) to be significant")
	}
	if len(res.TriggeredRules) != 2 {
		t.Fatalf("expected both corroborating metrics to be recorded as triggered rules, got %v", res.TriggeredRules)
	}
}

func TestEvaluate_PostEngagementsSpikeWithoutCorroboration_NotSignificant(t *testing.T) {
	signals := []model.Signal{{Source: "social_media", Type: "page_post_engagements", Value: 480}}
	baselines := map[string]memory.BaselineComparison{
		"social_media:page_post_engagements": {SeasonalZScore: 5.0},
		"social_media:page_engaged_users":    {SeasonalZScore: -0.1},
	}
	res := significance.Evaluate(signals, baselines, processor.GuardrailResult{})

	if res.IsSignificant {
		t.Fatalf("expected an uncorroborated post-engagements spike to stay insignificant, got true")
	}
}

func TestEvaluate_ReachSpikeWithMissingCompanionBaseline_FailsSafeToNotSignificant(t *testing.T) {
	signals := []model.Signal{{Source: "social_media", Type: "page_impressions", Value: 9600}}
	baselines := map[string]memory.BaselineComparison{
		"social_media:page_impressions": {SeasonalZScore: 6.0},
		// no page_engaged_users entry at all this window
	}
	res := significance.Evaluate(signals, baselines, processor.GuardrailResult{})

	if res.IsSignificant {
		t.Fatalf("expected a missing companion baseline to fail safe (not significant), got true")
	}
}

func TestEvaluate_UnrelatedMetricNeedsNoCorroboration(t *testing.T) {
	signals := []model.Signal{{Source: "weather", Type: "rain_mm", Value: 18}}
	baselines := map[string]memory.BaselineComparison{
		"weather:rain_mm": {SeasonalZScore: 3.1}, // not in corroborationPairs — should behave exactly as before
	}
	res := significance.Evaluate(signals, baselines, processor.GuardrailResult{})

	if !res.IsSignificant {
		t.Fatalf("expected an unrelated (non-social) metric to remain significant on its own, unaffected by the corroboration rule")
	}
}

func TestEvaluate_LowVolumeGuardrail_SuppressesFlaggedRatioMetric(t *testing.T) {
	signals := []model.Signal{{Source: "google_analytics", Type: "engagement_rate", Value: 100}}
	baselines := map[string]memory.BaselineComparison{
		// A tiny sample size can produce a huge, meaningless z-score on a ratio metric.
		"google_analytics:engagement_rate": {SeasonalZScore: 9.9},
	}
	guardrails := processor.GuardrailResult{
		IsLowVolume:       true,
		SuppressedMetrics: []string{"google_analytics:engagement_rate"},
	}
	res := significance.Evaluate(signals, baselines, guardrails)

	if res.IsSignificant {
		t.Fatalf("expected suppressed ratio metric under low volume to not drive significance, got true (score=%.2f)", res.Score)
	}
}
