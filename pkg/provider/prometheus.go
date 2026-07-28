package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

type PrometheusProvider struct {
	cfg    *config.Config
	client *http.Client
}

type prometheusQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"` // [timestamp, "value_string"]
		} `json:"result"`
	} `json:"data"`
}

func NewPrometheusProvider(cfg *config.Config) SignalProvider {
	return &PrometheusProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (p *PrometheusProvider) Name() string {
	return "Prometheus"
}

func (p *PrometheusProvider) PollFrequency() time.Duration {
	return 1 * time.Minute
}

func (p *PrometheusProvider) Fetch(ctx context.Context) ([]model.Signal, error) {
	if p.cfg.PrometheusURL == "" {
		log.Printf("[WARN] [Prometheus] PROMETHEUS_URL is not set. Skipping infrastructure telemetry extraction.")
		return nil, nil
	}

	queries := map[string]string{
		"system_cpu_utilization_pct":  `100 - (avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`,
		"system_memory_utilization_pct": `((node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes) * 100`,
		"http_requests_per_sec":         `sum(rate(http_requests_total[5m]))`,
		"http_5xx_error_rate_pct":       `(sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))) * 100`,
	}

	now := time.Now()
	var signals []model.Signal

	for metricType, query := range queries {
		val, err := p.queryPrometheus(ctx, query)
		if err != nil {
			log.Printf("[WARN] [Prometheus] Metric '%s' query failed or unreachable at %s: %v. Reporting baseline 0.0.", metricType, p.cfg.PrometheusURL, err)
			val = 0.0
		}

		unit := "%"
		if metricType == "http_requests_per_sec" {
			unit = "req/sec"
		}

		signals = append(signals, model.Signal{
			Source:          "prometheus",
			Type:            metricType,
			Value:           val,
			Unit:       unit,
			Confidence: 0.95,
			Timestamp:  now,
			Metadata: map[string]interface{}{
				"prometheusUrl": p.cfg.PrometheusURL,
				"promql":        query,
			},
			CreatedAt: now,
		})
	}

	log.Printf("[INFO] [Prometheus] Extracted %d infrastructure metrics from Prometheus", len(signals))
	return signals, nil
}

func (p *PrometheusProvider) queryPrometheus(ctx context.Context, promQL string) (float64, error) {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", p.cfg.PrometheusURL, url.QueryEscape(promQL))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0.0, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0.0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0.0, fmt.Errorf("Prometheus returned status %d", resp.StatusCode)
	}

	var res prometheusQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0.0, err
	}

	if res.Status != "success" || len(res.Data.Result) == 0 {
		return 0.0, nil
	}

	if len(res.Data.Result[0].Value) < 2 {
		return 0.0, nil
	}

	valStr, ok := res.Data.Result[0].Value[1].(string)
	if !ok {
		return 0.0, nil
	}

	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0.0, err
	}

	return val, nil
}
