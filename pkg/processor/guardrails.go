package processor

import (
	"fmt"
	"log"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

type GuardrailResult struct {
	IsLowVolume             bool     `json:"isLowVolume"`
	SuppressedMetrics       []string `json:"suppressedMetrics"`
	VolumeGuardrailPrompt   string   `json:"volumeGuardrailPrompt"`
}

// EvaluateVolumeGuardrails inspects signal volumes and suppresses volatile percentage ratios on low sample sizes.
func EvaluateVolumeGuardrails(signals []model.Signal) GuardrailResult {
	activeUsers := 0.0
	httpReqs := 0.0

	for _, s := range signals {
		if s.Source == "google_analytics" && s.Type == "active_users" {
			activeUsers = s.Value
		}
		if s.Source == "prometheus" && s.Type == "http_requests_per_sec" {
			httpReqs = s.Value
		}
	}

	isLowVolume := activeUsers < 10 && httpReqs < 1.0

	var suppressed []string
	if isLowVolume {
		suppressed = append(suppressed, "google_analytics:engagement_rate_pct", "google_analytics:bounce_rate_pct", "google_analytics:conversions")
		log.Printf("[INFO] [Guardrails] 🛡️ Low volume detected (ActiveUsers: %.0f, HTTP Req/sec: %.2f). Suppressing ratio metrics from triggering critical anomalies.", activeUsers, httpReqs)
	}

	promptAnnotation := ""
	if isLowVolume {
		promptAnnotation = fmt.Sprintf("\n[STATISTICAL GUARDRAIL WARNING]: Current user volume is low (ActiveUsers=%.0f, HTTP req/sec=%.2f). Percentage ratios (100%% engagement rate, 0 conversions) are statistically insignificant due to small sample size. DO NOT infer a Conversion Funnel Bottleneck or System Error.", activeUsers, httpReqs)
	}

	return GuardrailResult{
		IsLowVolume:           isLowVolume,
		SuppressedMetrics:     suppressed,
		VolumeGuardrailPrompt: promptAnnotation,
	}
}
