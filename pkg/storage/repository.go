package storage

import (
	"context"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

type SignalRepository interface {
	EnsureIndexes(ctx context.Context) error
	SaveSignals(ctx context.Context, signals []model.Signal) error
	GetRecentSignals(ctx context.Context, limit int) ([]model.Signal, error)
	GetSignalsInWindow(ctx context.Context, duration time.Duration) ([]model.Signal, error)
	SaveBusinessEvent(ctx context.Context, event *model.BusinessEvent) error
	GetBusinessTimeline(ctx context.Context, limit int) ([]model.BusinessEvent, error)
	// GetBusinessTimelineInWindow returns events with Timestamp in [start, end] —
	// added for scenario-conductor's report step (SIM-HB-01 §06), which needs
	// to grade a specific run's window, not "the most recent N regardless of
	// when they happened."
	GetBusinessTimelineInWindow(ctx context.Context, start, end time.Time) ([]model.BusinessEvent, error)
	GetBaselineProfile(ctx context.Context, metricKey string, dayOfWeek, hourOfDay int) (*model.BaselineProfile, error)
	UpdateBaselineProfile(ctx context.Context, profile *model.BaselineProfile) error
	Close(ctx context.Context) error
}
