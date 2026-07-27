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
