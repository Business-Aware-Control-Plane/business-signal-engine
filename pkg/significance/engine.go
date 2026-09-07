// Package significance implements the deterministic pre-filter described in
// BSAL-HB-01 §01: it decides whether a window of signals is business-significant
// enough to warrant an AI classification call, using only the dual-memory
// baselines (pkg/memory) and volume guardrails (pkg/processor) already computed
// for that window. It never itself decides *what* an event means — that stays
// with the AI layer — only *whether there is one worth naming at all*.
package significance

import (
	"fmt"
	"log"
	"math"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/memory"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/processor"
)

// SeasonalZThreshold is the |z| beyond which a metric's deviation from its own
// seasonal baseline counts as significant. Kept as a variable rather than a
// const so it can be treated as the tunable parameter the dissertation's
// evaluation methodology calibrates against a tuning-set (proposal §3.9.1),
// not a value hardcoded once and forgotten.
var SeasonalZThreshold = 2.0

// scheduledEventTypes are known-future business events — a public holiday, an
// active campaign window, a product launch — that are significant by
// definition. They are facts about the calendar, not statistical anomalies,
// so no z-score is meaningful for them; their mere presence (value > 0) is
// the signal.
var scheduledEventTypes = map[string]bool{
	"public_holiday":  true,
	"campaign_window": true,
	"product_launch":  true,
}

// Result is the deterministic verdict handed to CorrelationEngine. Score is
// the largest |seasonal z-score| observed across metrics in the window, kept
// signed (not absolute) so downstream logging/debugging can tell an increase
// from a decrease at a glance.
type Result struct {
	IsSignificant  bool
	Score          float64
	TriggeredRules []string
	DrivingMetrics []string
}

// Evaluate inspects the window and returns whether it is worth an AI call.
// Percentage-ratio metrics flagged by the volume guardrail as statistically
// insignificant on a low sample size never count toward significance here,
// mirroring the guardrail's own stated intent (pkg/processor/guardrails.go).
func Evaluate(signals []model.Signal, baselines map[string]memory.BaselineComparison, guardrails processor.GuardrailResult) Result {
	var res Result

	for _, s := range signals {
		if !scheduledEventTypes[s.Type] || s.Value <= 0 {
			continue
		}
		res.IsSignificant = true
		res.TriggeredRules = append(res.TriggeredRules, fmt.Sprintf("scheduled_event:%s:%s", s.Source, s.Type))
		res.DrivingMetrics = append(res.DrivingMetrics, fmt.Sprintf("%s:%s", s.Source, s.Type))
	}

	for key, b := range baselines {
		if guardrails.IsLowVolume && isSuppressed(key, guardrails) {
			continue
		}
		if math.Abs(b.SeasonalZScore) > math.Abs(res.Score) {
			res.Score = b.SeasonalZScore
		}
		if math.Abs(b.SeasonalZScore) > SeasonalZThreshold {
			res.IsSignificant = true
			res.TriggeredRules = append(res.TriggeredRules, fmt.Sprintf("seasonal_z:%s=%.2f", key, b.SeasonalZScore))
			res.DrivingMetrics = append(res.DrivingMetrics, key)
		}
	}

	if res.IsSignificant {
		log.Printf("[INFO] [SignificanceEngine] ⭐ Window flagged significant (maxZ=%.2f, rules=%v)", res.Score, res.TriggeredRules)
	} else {
		log.Printf("[DEBUG] [SignificanceEngine] Window not significant (maxZ=%.2f); AI call skipped", res.Score)
	}

	return res
}

func isSuppressed(key string, g processor.GuardrailResult) bool {
	for _, m := range g.SuppressedMetrics {
		if m == key {
			return true
		}
	}
	return false
}
