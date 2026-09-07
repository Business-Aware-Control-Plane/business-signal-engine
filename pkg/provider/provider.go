package provider

import (
	"context"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

// SignalProvider defines the contract for external and business metric data sources.
type SignalProvider interface {
	Name() string
	PollFrequency() time.Duration
	Fetch(ctx context.Context) ([]model.Signal, error)
}

// pollFrequencyOr returns override when it's set (>0), otherwise the
// provider's own production default. Every provider's PollFrequency()
// funnels through this so "unset config" and "explicit override" behave
// identically everywhere (SIM-HB-01 §10).
func pollFrequencyOr(override, productionDefault time.Duration) time.Duration {
	if override > 0 {
		return override
	}
	return productionDefault
}
