package memory

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/storage"
	"gonum.org/v1/gonum/stat"
)

type BaselineComparison struct {
	MetricKey        string  `json:"metricKey"`
	CurrentValue     float64 `json:"currentValue"`
	STMean24h        float64 `json:"stMean24h"`
	STStdDev24h      float64 `json:"stStdDev24h"`
	LTSeasonalMean   float64 `json:"ltSeasonalMean"`
	LTSeasonalStdDev float64 `json:"ltSeasonalStdDev"`
	SeasonalZScore   float64 `json:"seasonalZScore"`
	IsSeasonalNorm   bool    `json:"isSeasonalNorm"`
}

type MemoryEngine struct {
	repo storage.SignalRepository
}

func NewMemoryEngine(repo storage.SignalRepository) *MemoryEngine {
	return &MemoryEngine{repo: repo}
}

// EvaluateBaseline performs dual-memory (STM & LTM) comparison for active signals.
func (m *MemoryEngine) EvaluateBaseline(ctx context.Context, signals []model.Signal) map[string]BaselineComparison {
	results := make(map[string]BaselineComparison)
	if len(signals) == 0 {
		return results
	}

	now := time.Now()
	dayOfWeek := int(now.Weekday())
	hourOfDay := now.Hour()

	// 1. Fetch Short-Term Memory (STM) signals in rolling 24h window
	stmSignals, err := m.repo.GetSignalsInWindow(ctx, 24*time.Hour)
	if err != nil {
		log.Printf("[WARN] [MemoryEngine] Failed to fetch 24h STM signals: %v", err)
	}

	stmGrouped := make(map[string][]float64)
	for _, s := range stmSignals {
		key := fmt.Sprintf("%s:%s", s.Source, s.Type)
		stmGrouped[key] = append(stmGrouped[key], s.Value)
	}

	for _, s := range signals {
		key := fmt.Sprintf("%s:%s", s.Source, s.Type)

		// STM calculation
		values := stmGrouped[key]
		stMean := s.Value
		stStdDev := 0.0
		if len(values) > 1 {
			stMean = stat.Mean(values, nil)
			stStdDev = math.Sqrt(stat.Variance(values, nil))
		}

		// LTM Seasonal Profile Lookup
		profile, err := m.repo.GetBaselineProfile(ctx, key, dayOfWeek, hourOfDay)
		if err != nil {
			log.Printf("[WARN] [MemoryEngine] LTM lookup error for key '%s': %v", key, err)
		}

		ltMean := stMean
		ltStdDev := stStdDev
		if profile != nil && profile.SampleCount > 5 {
			ltMean = profile.MeanValue
			ltStdDev = profile.StdDevValue
		}

		seasonalZ := 0.0
		if ltStdDev > 0 {
			seasonalZ = (s.Value - ltMean) / ltStdDev
		}

		// Normal baseline condition: within 2.0 standard deviations of seasonal norm
		isSeasonalNorm := math.Abs(seasonalZ) < 2.0

		comp := BaselineComparison{
			MetricKey:        key,
			CurrentValue:     s.Value,
			STMean24h:        stMean,
			STStdDev24h:      stStdDev,
			LTSeasonalMean:   ltMean,
			LTSeasonalStdDev: ltStdDev,
			SeasonalZScore:   seasonalZ,
			IsSeasonalNorm:   isSeasonalNorm,
		}

		results[key] = comp

		log.Printf("[INFO] [MemoryEngine] '%s' ➔ Cur: %.2f | STM 24h: %.2f | LTM Seasonal (%d:%02d): %.2f ± %.2f | Z_seas: %.2f (Norm: %v)",
			key, s.Value, stMean, dayOfWeek, hourOfDay, ltMean, ltStdDev, seasonalZ, isSeasonalNorm)

		// Asynchronously update LTM seasonal profile
		go m.updateLTMProfile(key, dayOfWeek, hourOfDay, s.Value, profile)
	}

	return results
}

func (m *MemoryEngine) updateLTMProfile(metricKey string, dayOfWeek, hourOfDay int, currentVal float64, existing *model.BaselineProfile) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if existing == nil {
		newProf := &model.BaselineProfile{
			MetricKey:   metricKey,
			DayOfWeek:   dayOfWeek,
			HourOfDay:   hourOfDay,
			MeanValue:   currentVal,
			StdDevValue: 0.1,
			SampleCount: 1,
			LastUpdated: time.Now(),
		}
		_ = m.repo.UpdateBaselineProfile(ctx, newProf)
		return
	}

	// Exponential moving average update for seasonal baselines
	alpha := 0.1
	newCount := existing.SampleCount + 1
	newMean := (1-alpha)*existing.MeanValue + alpha*currentVal
	diff := currentVal - newMean
	newVar := (1-alpha)*math.Pow(existing.StdDevValue, 2) + alpha*math.Pow(diff, 2)
	newStdDev := math.Sqrt(newVar)

	existing.MeanValue = newMean
	existing.StdDevValue = newStdDev
	existing.SampleCount = newCount
	existing.LastUpdated = time.Now()

	_ = m.repo.UpdateBaselineProfile(ctx, existing)
}
