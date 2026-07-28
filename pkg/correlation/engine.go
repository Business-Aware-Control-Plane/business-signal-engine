package correlation

import (
	"context"
	"log"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/ai"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

type CorrelationEngine struct {
	aiService *ai.AIService
}

func NewCorrelationEngine(aiService *ai.AIService) *CorrelationEngine {
	return &CorrelationEngine{aiService: aiService}
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

	// Pattern Rule Check 1: Weather + Active Users
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
	}

	// Invoke AI service for correlation synthesis and reasoning
	aiRes, err := e.aiService.AnalyzeSignalsAndCorrelate(ctx, signals, triggeredRules)
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
		},
	}

	log.Printf("[INFO] [CorrelationEngine] Generated BusinessEvent '%s' (Category: %s, Severity: %s, Confidence: %.2f)", event.EventType, event.Category, event.Severity, event.Confidence)
	return event, nil
}
