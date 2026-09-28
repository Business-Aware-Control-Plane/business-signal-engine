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

// corroborationPairs names metrics whose own significance is not trusted in
// isolation — each requires its paired "companion" metric to also show at
// least CorroborationZThreshold of deviation in the same window before the
// first metric counts toward IsSignificant.
//
// This exists specifically for reach-style social metrics (impressions,
// post engagements): a real viral moment lifts unique engaged users right
// alongside reach, but bot/inorganic amplification inflates reach without a
// proportional lift in unique engagement — the one distinguishing signal
// available *at detection time*, unlike "did real traffic eventually
// follow," which isn't knowable yet and so can never be a legitimate basis
// for a significance rule (see SIM-HB-01 §08's Scenario 3 vs. 6 finding:
// both scenarios have an identical business-track input, differing only in
// what happens afterward — no rule keyed on future traffic could ever
// distinguish them without breaking Scenario 3's whole reason to exist,
// early detection ahead of real demand).
var corroborationPairs = map[string]string{
	"social_media:page_impressions":      "social_media:page_engaged_users",
	"social_media:page_post_engagements": "social_media:page_engaged_users",
}

// CorroborationZThreshold is deliberately lower than SeasonalZThreshold — the
// companion metric only needs to show it moved too, not independently clear
// the full significance bar on its own.
var CorroborationZThreshold = 1.0

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

	var uncorroborated []string
	for key, b := range baselines {
		if guardrails.IsLowVolume && isSuppressed(key, guardrails) {
			continue
		}
		if math.Abs(b.SeasonalZScore) > math.Abs(res.Score) {
			res.Score = b.SeasonalZScore
		}
		if math.Abs(b.SeasonalZScore) > SeasonalZThreshold {
			if companion, needsCorroboration := corroborationPairs[key]; needsCorroboration && !corroborates(companion, baselines) {
				uncorroborated = append(uncorroborated, fmt.Sprintf("%s=%.2f (needs %s)", key, b.SeasonalZScore, companion))
				continue
			}
			res.IsSignificant = true
			res.TriggeredRules = append(res.TriggeredRules, fmt.Sprintf("seasonal_z:%s=%.2f", key, b.SeasonalZScore))
			res.DrivingMetrics = append(res.DrivingMetrics, key)
		}
	}

	if len(uncorroborated) > 0 && !res.IsSignificant {
		log.Printf("[DEBUG] [SignificanceEngine] Suppressed uncorroborated reach spike(s): %v", uncorroborated)
	}

	if res.IsSignificant {
		log.Printf("[INFO] [SignificanceEngine] ⭐ Window flagged significant (maxZ=%.2f, rules=%v)", res.Score, res.TriggeredRules)
	} else {
		log.Printf("[DEBUG] [SignificanceEngine] Window not significant (maxZ=%.2f); AI call skipped", res.Score)
	}

	return res
}

// corroborates reports whether companionKey's own baseline shows at least
// CorroborationZThreshold of deviation — absent entirely (e.g. suppressed by
// a low-volume guardrail, or genuinely never observed this window) counts as
// "does not corroborate," a fail-safe default rather than assuming the best.
func corroborates(companionKey string, baselines map[string]memory.BaselineComparison) bool {
	companion, ok := baselines[companionKey]
	if !ok {
		return false
	}
	return math.Abs(companion.SeasonalZScore) >= CorroborationZThreshold
}

func isSuppressed(key string, g processor.GuardrailResult) bool {
	for _, m := range g.SuppressedMetrics {
		if m == key {
			return true
		}
	}
	return false
}
