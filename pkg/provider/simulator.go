package provider

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
)

// SimulatorProvider is a dedicated test stream module for simulating metrics during development and testing.
type SimulatorProvider struct {
	cfg *config.Config
}

func NewSimulatorProvider(cfg *config.Config) *SimulatorProvider {
	return &SimulatorProvider{cfg: cfg}
}

func (p *SimulatorProvider) Name() string {
	return "SimulatorStream"
}

func (p *SimulatorProvider) PollFrequency() time.Duration {
	return 10 * time.Second // Fast poll frequency for testing stream
}

func (p *SimulatorProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	now := time.Now()
	var signals []model.Signal

	// 1. Simulate Google Analytics metrics
	activeUsers := float64(450 + rand.Intn(150))
	sessionDuration := float64(180 + rand.Intn(60))
	signals = append(signals,
		model.Signal{
			SignalID:   uuid.New().String(),
			Source:     "simulated_google_analytics",
			Type:       "active_users",
			Value:      activeUsers,
			Unit:       "count",
			Confidence: 0.90,
			Metadata: map[string]interface{}{
				"simulated": true,
				"stream":    "test_simulation",
			},
			Timestamp: now,
		},
		model.Signal{
			SignalID:   uuid.New().String(),
			Source:     "simulated_google_analytics",
			Type:       "avg_session_duration",
			Value:      sessionDuration,
			Unit:       "seconds",
			Confidence: 0.90,
			Metadata: map[string]interface{}{
				"simulated": true,
				"stream":    "test_simulation",
			},
			Timestamp: now,
		},
	)

	// 2. Simulate Meta Business Ads metrics
	ctr := 3.5 + (rand.Float64() * 1.2)
	adSpend := float64(300 + rand.Intn(100))
	signals = append(signals,
		model.Signal{
			SignalID:   uuid.New().String(),
			Source:     "simulated_meta_business",
			Type:       "ad_ctr_pct",
			Value:      ctr,
			Unit:       "percentage",
			Confidence: 0.88,
			Metadata: map[string]interface{}{
				"simulated": true,
				"campaign":  "Simulated Sri Lanka Promo",
			},
			Timestamp: now,
		},
		model.Signal{
			SignalID:   uuid.New().String(),
			Source:     "simulated_meta_business",
			Type:       "ad_spend_usd",
			Value:      adSpend,
			Unit:       "USD",
			Confidence: 0.95,
			Metadata: map[string]interface{}{
				"simulated": true,
			},
			Timestamp: now,
		},
	)

	// 3. Simulate Weather metrics
	temp := 28.0 + (rand.Float64() * 4.0)
	rain := rand.Float64() * 15.0
	signals = append(signals,
		model.Signal{
			SignalID:   uuid.New().String(),
			Source:     "simulated_weather",
			Type:       "temperature",
			Value:      temp,
			Unit:       "celsius",
			Confidence: 0.85,
			Metadata: map[string]interface{}{
				"simulated": true,
				"country":   p.cfg.CountryCode,
			},
			Timestamp: now,
		},
		model.Signal{
			SignalID:   uuid.New().String(),
			Source:     "simulated_weather",
			Type:       "rain_mm",
			Value:      rain,
			Unit:       "mm",
			Confidence: 0.85,
			Metadata: map[string]interface{}{
				"simulated":   true,
				"isHeavyRain": rain > 10.0,
			},
			Timestamp: now,
		},
	)

	log.Printf("[INFO] [SimulatorStream] Extracted %d simulated test stream signals", len(signals))
	return signals, nil
}
