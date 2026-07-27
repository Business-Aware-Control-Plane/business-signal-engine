package storage

import (
	"context"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

// SignalRepository defines storage operations for business signals.
type SignalRepository interface {
	EnsureIndexes(ctx context.Context) error
	SaveSignals(ctx context.Context, signals []model.Signal) error
	GetRecentSignals(ctx context.Context, limit int) ([]model.Signal, error)
	Close(ctx context.Context) error
}
