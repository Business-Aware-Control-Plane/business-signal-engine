package correlation

import (
	"context"
	"log"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/ai"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/memory"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/processor"
)

type CorrelationEngine struct {
	aiService    *ai.AIService
	memoryEngine *memory.MemoryEngine
}

func NewCorrelationEngine(aiService *ai.AIService, memoryEngine *memory.MemoryEngine) *CorrelationEngine {
	return &CorrelationEngine{
		aiService:    aiService,
		memoryEngine: memoryEngine,
	}
}

func (e *CorrelationEngine) Evaluate(ctx context.Context, window model.TimeWindow, signals []model.Signal) (*model.BusinessEvent, error) {
	if len(signals) == 0 {
		return nil, nil
	}

	var triggeredRules []string
	var sources []string
	sourceSet := make(map[string]bool)

	for _, s := range signals {
		if !sourceSet[s.Source] {
			sourceSet[s.Source] = true
			sources = append(sources, s.Source)
		}
	}

	// 1. Dual-Memory Baseline Evaluation (STM 24h & LTM Seasonal)
	baselines := e.memoryEngine.EvaluateBaseline(ctx, signals)

	// 2. Statistical Volume Guardrails Pre-check
	guardrails := processor.EvaluateVolumeGuardrails(signals)

	// 3. Rule Checks
	for _, s := range signals {
		if s.Source == "weather" && s.Type == "rain_mm" && s.Value > 2.0 {
			triggeredRules = append(triggeredRules, "Rule_Rain_Precipitation_Active")
		}
		if s.Source == "google_analytics" && s.Type == "active_users" && s.Value > 300 {
			triggeredRules = append(triggeredRules, "Rule_GA4_Traffic_Surge")
		}
		if s.Source == "meta_business" && s.Type == "ad_ctr_pct" && s.Value > 3.0 {
			triggeredRules = append(triggeredRules, "Rule_Meta_CTR_High")
		}
		if s.Source == "prometheus" && s.Type == "http_5xx_error_rate_pct" && s.Value > 5.0 {
			triggeredRules = append(triggeredRules, "Rule_Prometheus_High_5xx_Errors")
		}
	}

	// 4. Invoke AI Service with Baselines & Guardrails
	aiRes, err := e.aiService.AnalyzeSignalsAndCorrelate(ctx, signals, baselines, guardrails, triggeredRules)
	if err != nil {
		log.Printf("[WARN] [CorrelationEngine] AI analysis error: %v", err)
	}

	if aiRes == nil {
		return nil, nil
	}

	// Build BusinessEvent model
	event := &model.BusinessEvent{
		EventType:         aiRes.EventType,
		Category:          aiRes.Category,
		Severity:          aiRes.Severity,
		Confidence:        aiRes.Confidence,
		TimeWindow:        window,
		SupportingSignals: sources,
		AISummary:         aiRes.AISummary,
		Metadata: map[string]interface{}{
			"triggeredRules": triggeredRules,
			"signalCount":    len(signals),
			"isLowVolume":    guardrails.IsLowVolume,
		},
	}

	log.Printf("[INFO] [CorrelationEngine] Generated BusinessEvent '%s' (Category: %s, Severity: %s, Confidence: %.2f)", event.EventType, event.Category, event.Severity, event.Confidence)
	return event, nil
}
