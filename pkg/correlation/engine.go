package correlation

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/ai"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/memory"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/processor"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/significance"
)

// DefaultSuppressionWindow is how long an identical (eventType, category)
// pair is suppressed from being re-published once seen, per BSAL-HB-01 §02.
// Exported so it can be recalibrated against the tuning-set scenarios in
// proposal §3.9.1 rather than treated as a magic constant.
const DefaultSuppressionWindow = 10 * time.Minute

type CorrelationEngine struct {
	aiService         *ai.AIService
	memoryEngine      *memory.MemoryEngine
	SuppressionWindow time.Duration

	mu            sync.Mutex
	lastPublished map[string]time.Time // "eventType|category" -> last time it was let through
}

func NewCorrelationEngine(aiService *ai.AIService, memoryEngine *memory.MemoryEngine) *CorrelationEngine {
	return &CorrelationEngine{
		aiService:         aiService,
		memoryEngine:      memoryEngine,
		SuppressionWindow: DefaultSuppressionWindow,
		lastPublished:     make(map[string]time.Time),
	}
}

// Evaluate returns (nil, nil) whenever the window is not worth surfacing —
// either because it failed the significance gate (§01) or because the
// resulting classification landed on Low/NormalBusinessActivity or a recent
// duplicate anyway (§02). A non-nil return is the BSAL's positive claim that
// this is a business event worth the AI Control Plane's attention.
func (e *CorrelationEngine) Evaluate(ctx context.Context, window model.TimeWindow, signals []model.Signal) (*model.BusinessEvent, error) {
	if len(signals) == 0 {
		return nil, nil
	}

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

	// 3. Deterministic significance gate — the AI layer is never invoked on a
	//    window this fails, closing both the cost problem and the RQ1 ablation
	//    problem identified in BSAL-HB-01 §01.
	sig := significance.Evaluate(signals, baselines, guardrails)
	if !sig.IsSignificant {
		return nil, nil
	}

	// 4. Invoke AI Service — narrowed to classification + narrative over an
	//    already-flagged window, not detection from scratch.
	aiRes, err := e.aiService.AnalyzeSignalsAndCorrelate(ctx, signals, baselines, guardrails, sig.TriggeredRules, sig.DrivingMetrics)
	if err != nil {
		log.Printf("[WARN] [CorrelationEngine] AI analysis error: %v", err)
	}
	if aiRes == nil {
		return nil, nil
	}

	// 5. Publish-significance safety net (§02): even a window the gate flagged
	//    can still land on a normal-activity classification once the AI layer
	//    looks closer — that's not an event either.
	if aiRes.Severity == "Low" && aiRes.EventType == "NormalBusinessActivity" {
		log.Printf("[DEBUG] [CorrelationEngine] Classification resolved to NormalBusinessActivity/Low despite significance flag; suppressing")
		return nil, nil
	}

	// 6. Suppression window: don't re-surface the same (eventType, category)
	//    pair while it's still active.
	dedupKey := aiRes.EventType + "|" + aiRes.Category
	now := time.Now()
	e.mu.Lock()
	if last, ok := e.lastPublished[dedupKey]; ok && now.Sub(last) < e.SuppressionWindow {
		e.mu.Unlock()
		log.Printf("[DEBUG] [CorrelationEngine] Suppressing repeat of '%s' (last surfaced %s ago, window=%s)", dedupKey, now.Sub(last).Round(time.Second), e.SuppressionWindow)
		return nil, nil
	}
	e.lastPublished[dedupKey] = now
	e.mu.Unlock()

	event := &model.BusinessEvent{
		EventType:         aiRes.EventType,
		Category:          aiRes.Category,
		Severity:          aiRes.Severity,
		Confidence:        aiRes.Confidence,
		TimeWindow:        window,
		SupportingSignals: sources,
		AISummary:         aiRes.AISummary,
		Metadata: map[string]interface{}{
			"triggeredRules": sig.TriggeredRules,
			"drivingMetrics": sig.DrivingMetrics,
			"significanceZ":  sig.Score,
			"signalCount":    len(signals),
			"isLowVolume":    guardrails.IsLowVolume,
		},
	}

	log.Printf("[INFO] [CorrelationEngine] Generated BusinessEvent '%s' (Category: %s, Severity: %s, Confidence: %.2f)", event.EventType, event.Category, event.Severity, event.Confidence)
	return event, nil
}
