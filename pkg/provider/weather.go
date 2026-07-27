package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/google/uuid"
)

type WeatherProvider struct {
	cfg        *config.Config
	httpClient *http.Client
}

type openMeteoResponse struct {
	Current struct {
		Time             string  `json:"time"`
		Temperature2m    float64 `json:"temperature_2m"`
		RelativeHumidity float64 `json:"relative_humidity_2m"`
		Precipitation    float64 `json:"precipitation"`
		Rain             float64 `json:"rain"`
		WeatherCode      float64 `json:"weather_code"`
		WindSpeed10m     float64 `json:"wind_speed_10m"`
	} `json:"current"`
}

func NewWeatherProvider(cfg *config.Config) *WeatherProvider {
	return &WeatherProvider{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *WeatherProvider) Name() string {
	return "Weather"
}

func (p *WeatherProvider) PollFrequency() time.Duration {
	return 15 * time.Minute
}

func (p *WeatherProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,relative_humidity_2m,precipitation,rain,weather_code,wind_speed_10m",
		p.cfg.Latitude,
		p.cfg.Longitude,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create weather request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("[WARN] Weather API unreachable: %v. Skipping extraction.", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARN] Weather API returned status %d. Skipping extraction.", resp.StatusCode)
		return nil, fmt.Errorf("weather API returned status code %d", resp.StatusCode)
	}

	var data openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode weather response: %w", err)
	}

	now := time.Now()
	signals := []model.Signal{
		{
			SignalID:   uuid.New().String(),
			Source:     "weather",
			Type:       "temperature",
			Value:      data.Current.Temperature2m,
			Unit:       "celsius",
			Confidence: 0.98,
			Metadata: map[string]interface{}{
				"latitude":  p.cfg.Latitude,
				"longitude": p.cfg.Longitude,
				"country":   p.cfg.CountryCode,
			},
			Timestamp: now,
		},
		{
			SignalID:   uuid.New().String(),
			Source:     "weather",
			Type:       "rain_mm",
			Value:      data.Current.Rain,
			Unit:       "mm",
			Confidence: 0.95,
			Metadata: map[string]interface{}{
				"precipitation": data.Current.Precipitation,
				"weatherCode":   data.Current.WeatherCode,
				"isHeavyRain":   data.Current.Rain > 10.0,
			},
			Timestamp: now,
		},
		{
			SignalID:   uuid.New().String(),
			Source:     "weather",
			Type:       "humidity_pct",
			Value:      data.Current.RelativeHumidity,
			Unit:       "percentage",
			Confidence: 0.95,
			Metadata: map[string]interface{}{
				"windSpeedKmH": data.Current.WindSpeed10m,
			},
			Timestamp: now,
		},
	}

	log.Printf("[INFO] [Weather] Extracted %d signals for Sri Lanka (Temp: %.1f C, Rain: %.1f mm)", len(signals), data.Current.Temperature2m, data.Current.Rain)
	return signals, nil
}
