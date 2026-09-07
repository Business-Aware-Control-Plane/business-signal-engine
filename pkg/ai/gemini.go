package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/memory"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/processor"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type AIService struct {
	cfg    *config.Config
	client *genai.Client
}

type AIAnalysisResult struct {
	EventType  string  `json:"eventType"`  // e.g. "ExpectedDemandIncrease", "ViralMarketingSpike", "NormalBusinessActivity"
	Category   string  `json:"category"`   // e.g. "Marketing", "Environmental", "Operational", "Business Calendar"
	Severity   string  `json:"severity"`   // "Low", "Medium", "High", "Critical"
	Confidence float64 `json:"confidence"` // 0.0 to 1.0
	AISummary  string  `json:"aiSummary"`  // Rich narrative rationale
}

func NewAIService(ctx context.Context, cfg *config.Config) (*AIService, error) {
	if cfg.GeminiAPIKey == "" {
		log.Printf("[INFO] [AIService] GEMINI_API_KEY not configured. Rule-based AI synthesis fallback active.")
		return &AIService{cfg: cfg}, nil
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiAPIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Gemini AI client: %w", err)
	}

	log.Printf("[INFO] [AIService] Google Gemini AI Service initialized successfully (Model: %s)", cfg.GeminiModel)
	return &AIService{cfg: cfg, client: client}, nil
}

func (a *AIService) AnalyzeSignalsAndCorrelate(
	ctx context.Context,
	signals []model.Signal,
	baselines map[string]memory.BaselineComparison,
	guardrails processor.GuardrailResult,
	rulesTriggered []string,
	drivingMetrics []string,
) (*AIAnalysisResult, error) {
	if len(signals) == 0 {
		return nil, nil
	}

	if a.client == nil {
		log.Printf("[INFO] [AIService] Running deterministic fallback AI synthesis on %d active signals", len(signals))
		return a.fallbackCorrelation(drivingMetrics, guardrails, rulesTriggered), nil
	}

	modelName := a.cfg.GeminiModel
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}

	log.Printf("[INFO] [AIService] 🤖 Invoking Gemini AI model '%s' to analyze %d signals with Dual-Memory Baselines & Guardrails...", modelName, len(signals))

	modelClient := a.client.GenerativeModel(modelName)
	modelClient.SetTemperature(0.2) // Low temperature for deterministic classification

	prompt := fmt.Sprintf(`SYSTEM INSTRUCTIONS & GUARDRAILS:
You are an expert AIOps engine for cloud-native applications in Sri Lanka.
This window has already been deterministically flagged as statistically significant
by a rule- and baseline-driven pre-filter (see TRIGGERED RULES below) — your job is
to classify and narrate what kind of event this is, not to decide from scratch
whether one exists at all.
CRITICAL ZERO-HALLUCINATION RULES:
1. NEVER infer a "Conversion Funnel Bottleneck", "System Error", or "Critical Anomaly" when active user traffic or HTTP request volume is low (%s). Low traffic during off-peak hours is NORMAL baseline behavior.
2. Cross-reference external user traffic with internal Prometheus infrastructure telemetry (HTTP 5xx error rate, CPU utilization). If 5xx errors are 0%% and CPU is under baseline, classify system status as "NormalBusinessActivity" with Severity "Low".
3. Evaluate metric values strictly against the provided Short-Term (24h) and Long-Term Seasonal baselines.

ACTIVE SIGNALS:
%s

DUAL-MEMORY BASELINES (STM 24h & LTM Seasonal):
%s

STATISTICAL GUARDRAILS:
%s

TRIGGERED RULES:
%v

DRIVING METRICS (the specific signals that actually caused this window to be flagged —
classify based on these, not on other signals merely present in the same window):
%v

TASK:
Return a JSON object with:
- "eventType": (string, e.g. "NormalBusinessActivity", "ExpectedDemandIncrease", "WeatherDemandShift", "ViralCampaignSpike")
- "category": (string, "Operational", "Marketing", "Environmental", or "Business Calendar")
- "severity": (string, "Low", "Medium", "High", or "Critical")
- "confidence": (number between 0.0 and 1.0)
- "aiSummary": (string explanation strictly adhering to the guardrail rules)

Return ONLY valid JSON matching this schema.`,
		guardrails.VolumeGuardrailPrompt,
		formatSignalsForPrompt(signals),
		formatBaselinesForPrompt(baselines),
		guardrails.VolumeGuardrailPrompt,
		rulesTriggered,
		drivingMetrics,
	)

	resp, err := modelClient.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Printf("[WARN] [AIService] Gemini API call failed: %v. Using fallback synthesis.", err)
		return a.fallbackCorrelation(drivingMetrics, guardrails, rulesTriggered), nil
	}

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		var sb strings.Builder
		for _, part := range resp.Candidates[0].Content.Parts {
			sb.WriteString(fmt.Sprintf("%v", part))
		}
		rawJSON := cleanJSONResponse(sb.String())

		var res AIAnalysisResult
		if err := json.Unmarshal([]byte(rawJSON), &res); err == nil {
			log.Printf("[INFO] [AIService] 🧠 Gemini AI Correlation Analysis Output:")
			log.Printf("       ├─ EventType:  %s", res.EventType)
			log.Printf("       ├─ Category:   %s", res.Category)
			log.Printf("       ├─ Severity:   %s", res.Severity)
			log.Printf("       ├─ Confidence: %.2f", res.Confidence)
			log.Printf("       └─ AI Summary: \"%s\"", res.AISummary)
			return &res, nil
		} else {
			log.Printf("[WARN] [AIService] Failed to parse JSON response from Gemini AI: %v. Raw text: %s", err, rawJSON)
		}
	}

	return a.fallbackCorrelation(drivingMetrics, guardrails, rulesTriggered), nil
}

// fallbackCorrelation classifies a window the significance gate has already
// flagged (pkg/significance) — CorrelationEngine only reaches this path once
// IsSignificant is true, so this function's job is purely "what kind of
// event is this," never "is there an event at all."
//
// It classifies strictly from drivingMetrics — the specific "source:type"
// keys the significance engine identified as the actual cause — rather than
// scanning every signal present in the window. Two prior designs were both
// wrong in ways this review caught: matching on hardcoded exact Source
// strings ("weather", "google_analytics") missed the simulator's
// "simulated_*" sources and the newer social_media/business_calendar/stripe
// providers entirely (silently falling through to NormalBusinessActivity/Low
// even when correctly flagged significant); and a later fix that matched on
// Type but scanned the *whole* signal set made classification depend on
// incidental unrelated values (e.g. rain_mm happening to roll above 5mm)
// that had nothing to do with what was actually significant, producing a
// different eventType on every run for the same underlying cause — which
// silently defeated the §02 suppression window (keyed on eventType+category)
// and worked against the reproducibility this deterministic fallback exists
// for in the first place (proposal §3.4.3 / NFR-R1).
func (a *AIService) fallbackCorrelation(drivingMetrics []string, guardrails processor.GuardrailResult, rulesTriggered []string) *AIAnalysisResult {
	if len(rulesTriggered) == 0 {
		// Defensive only: nothing in the current call path reaches this
		// function without the significance gate already having fired, but
		// this keeps it safe to call standalone (e.g. in tests) too.
		return &AIAnalysisResult{
			EventType:  "NormalBusinessActivity",
			Category:   "Operational",
			Severity:   "Low",
			Confidence: 0.95,
			AISummary:  "System operating normally under baseline parameters.",
		}
	}

	hasRain := false
	hasHighTraffic := false
	hasAdActivity := false
	hasScheduledEvent := false

	for _, m := range drivingMetrics {
		metricType := m
		if idx := strings.LastIndex(m, ":"); idx != -1 {
			metricType = m[idx+1:]
		}
		switch metricType {
		case "rain_mm":
			hasRain = true
		case "active_users":
			hasHighTraffic = true
		case "ad_spend_usd", "ad_ctr_pct", "page_post_engagements", "page_engaged_users":
			hasAdActivity = true
		}
	}
	for _, r := range rulesTriggered {
		if strings.HasPrefix(r, "scheduled_event:") {
			hasScheduledEvent = true
		}
	}

	eventType := "SignificantDeviationDetected"
	category := "Operational"
	severity := "Medium"
	summary := fmt.Sprintf("Statistically significant deviation from seasonal baseline detected (%s); classified deterministically, Gemini AI unavailable.", strings.Join(rulesTriggered, ", "))

	switch {
	case hasScheduledEvent:
		eventType = "ScheduledBusinessEvent"
		category = "Business Calendar"
		severity = "High"
		summary = "A known scheduled business event (public holiday, campaign window, or product launch) is active or imminent."
	case hasRain && hasHighTraffic:
		eventType = "WeatherDemandShift"
		category = "Environmental"
		severity = "High"
		summary = "Heavy rainfall combined with high user activity predicts increased demand for ride/delivery operations."
	case hasAdActivity || hasHighTraffic:
		eventType = "MarketingMomentumSurge"
		category = "Marketing"
		severity = "Medium"
		summary = "Active marketing/engagement activity coincides with a statistically significant deviation from baseline."
	}

	if guardrails.IsLowVolume {
		// Low volume no longer overrides significance outright (the gate
		// already accounted for it when deciding to invoke this function at
		// all) — it only tempers confidence, since a ratio-based signal is
		// less trustworthy on a small sample even when a count-based one
		// (like active_users itself) is what actually triggered the rule.
		severity = "Medium"
	}

	res := &AIAnalysisResult{
		EventType:  eventType,
		Category:   category,
		Severity:   severity,
		Confidence: 0.85,
		AISummary:  summary,
	}

	log.Printf("[INFO] [AIService] ⚙️ Rule-Based Fallback Synthesis Output: EventType='%s', Severity='%s', Category='%s'", res.EventType, res.Severity, res.Category)
	return res
}

func formatSignalsForPrompt(signals []model.Signal) string {
	var parts []string
	for _, s := range signals {
		parts = append(parts, fmt.Sprintf("- %s:%s = %.2f (%s)", s.Source, s.Type, s.Value, s.Unit))
	}
	return strings.Join(parts, "\n")
}

func formatBaselinesForPrompt(baselines map[string]memory.BaselineComparison) string {
	var parts []string
	for key, b := range baselines {
		parts = append(parts, fmt.Sprintf("- %s: Current=%.2f | STM 24h Mean=%.2f | LTM Seasonal Mean=%.2f (Z_seasonal=%.2f, Norm=%v)",
			key, b.CurrentValue, b.STMean24h, b.LTSeasonalMean, b.SeasonalZScore, b.IsSeasonalNorm))
	}
	return strings.Join(parts, "\n")
}

func cleanJSONResponse(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}

func (a *AIService) Close() {
	if a.client != nil {
		a.client.Close()
	}
}
