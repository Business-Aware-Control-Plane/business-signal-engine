package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type AIService struct {
	cfg    *config.Config
	client *genai.Client
}

type AIAnalysisResult struct {
	EventType  string  `json:"eventType"`  // e.g. "ExpectedDemandIncrease", "ViralMarketingSpike"
	Category   string  `json:"category"`   // e.g. "Marketing", "Environmental", "Operational"
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

func (a *AIService) AnalyzeSignalsAndCorrelate(ctx context.Context, signals []model.Signal, rulesTriggered []string) (*AIAnalysisResult, error) {
	if len(signals) == 0 {
		return nil, nil
	}

	if a.client == nil {
		log.Printf("[INFO] [AIService] Running deterministic fallback AI synthesis on %d active signals", len(signals))
		return a.fallbackCorrelation(signals, rulesTriggered), nil
	}

	modelName := a.cfg.GeminiModel
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}

	log.Printf("[INFO] [AIService] 🤖 Invoking Gemini AI model '%s' to analyze %d signals and %d triggered correlation rules...", modelName, len(signals), len(rulesTriggered))

	modelClient := a.client.GenerativeModel(modelName)
	modelClient.SetTemperature(0.2) // Low temperature for deterministic classification

	prompt := fmt.Sprintf(`Analyze the following real-time business and external signals and evaluate potential business impact for a cloud-native platform in Sri Lanka:

Active Signals: %v
Triggered Correlation Rules: %v

Task: Synthesize these signals and return a JSON object with:
- "eventType": (string, e.g. "ExpectedDemandIncrease", "WeatherDisruption", "ViralCampaignSpike")
- "category": (string, e.g. "Marketing", "Environmental", "Operational", "Business Calendar")
- "severity": (string, "Low", "Medium", "High", or "Critical")
- "confidence": (number between 0.0 and 1.0)
- "aiSummary": (string explanation of the business context and predicted impact)

Return ONLY valid JSON matching this schema.`, formatSignalsForPrompt(signals), rulesTriggered)

	resp, err := modelClient.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Printf("[WARN] [AIService] Gemini API call failed: %v. Using fallback synthesis.", err)
		return a.fallbackCorrelation(signals, rulesTriggered), nil
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

	return a.fallbackCorrelation(signals, rulesTriggered), nil
}

func (a *AIService) fallbackCorrelation(signals []model.Signal, rulesTriggered []string) *AIAnalysisResult {
	hasRain := false
	hasHighGA := false
	hasHighMeta := false
	hasHoliday := false

	for _, s := range signals {
		if s.Source == "weather" && s.Type == "rain_mm" && s.Value > 5.0 {
			hasRain = true
		}
		if s.Source == "google_analytics" && s.Type == "active_users" && s.Value > 400 {
			hasHighGA = true
		}
		if s.Source == "meta_business" && s.Type == "ad_spend_usd" && s.Value > 200 {
			hasHighMeta = true
		}
		if s.Source == "calendar" && s.Type == "public_holiday" && s.Value > 0 {
			hasHoliday = true
		}
	}

	severity := "Low"
	eventType := "NormalBusinessActivity"
	category := "Operational"
	summary := "System operating under baseline parameters."

	if hasRain && (hasHighGA || hasHoliday) {
		eventType = "WeatherDemandShift"
		category = "Environmental"
		severity = "High"
		summary = "Heavy rainfall combined with high user activity predicts increased demand for ride/delivery operations."
	} else if hasHighMeta || hasHighGA {
		eventType = "MarketingMomentumSurge"
		category = "Marketing"
		severity = "Medium"
		summary = "Active marketing campaign and web traffic surge indicating elevated application load."
	}

	res := &AIAnalysisResult{
		EventType:  eventType,
		Category:   category,
		Severity:   severity,
		Confidence: 0.90,
		AISummary:  summary,
	}

	log.Printf("[INFO] [AIService] ⚙️ Rule-Based Fallback Synthesis Output: EventType='%s', Severity='%s', Category='%s'", res.EventType, res.Severity, res.Category)
	return res
}

func formatSignalsForPrompt(signals []model.Signal) string {
	var parts []string
	for _, s := range signals {
		parts = append(parts, fmt.Sprintf("%s:%s=%.2f(%s)", s.Source, s.Type, s.Value, s.Unit))
	}
	return strings.Join(parts, ", ")
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
