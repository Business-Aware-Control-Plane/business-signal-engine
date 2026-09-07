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
