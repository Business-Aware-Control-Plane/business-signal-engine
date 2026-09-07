package pipeline_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/ai"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/config"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/correlation"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/memory"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/pipeline"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/provider"
)

// fakeRepo is a minimal, thread-safe, in-memory implementation of
// storage.SignalRepository, so the real pipeline (Collector, CorrelationEngine,
// MemoryEngine, SignificanceEngine) can be exercised end-to-end without a
// live MongoDB — this is what actually proves the wiring in main.go is
// correct, not just each package in isolation.
type fakeRepo struct {
	mu        sync.Mutex
	signals   []model.Signal
	events    []model.BusinessEvent
	baselines map[string]*model.BaselineProfile
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{baselines: make(map[string]*model.BaselineProfile)}
}

func (r *fakeRepo) EnsureIndexes(ctx context.Context) error { return nil }

func (r *fakeRepo) SaveSignals(ctx context.Context, signals []model.Signal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = append(r.signals, signals...)
	return nil
}

func (r *fakeRepo) GetRecentSignals(ctx context.Context, limit int) ([]model.Signal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit > len(r.signals) {
		limit = len(r.signals)
	}
	return append([]model.Signal{}, r.signals[len(r.signals)-limit:]...), nil
}

func (r *fakeRepo) GetSignalsInWindow(ctx context.Context, duration time.Duration) ([]model.Signal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	since := time.Now().Add(-duration)
	var out []model.Signal
	for _, s := range r.signals {
		if s.Timestamp.After(since) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *fakeRepo) SaveBusinessEvent(ctx context.Context, event *model.BusinessEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, *event)
	return nil
}

func (r *fakeRepo) GetBusinessTimeline(ctx context.Context, limit int) ([]model.BusinessEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]model.BusinessEvent{}, r.events...), nil
}

func baselineKey(metricKey string, dayOfWeek, hourOfDay int) string {
	return fmt.Sprintf("%s|%d|%d", metricKey, dayOfWeek, hourOfDay)
}

func (r *fakeRepo) GetBaselineProfile(ctx context.Context, metricKey string, dayOfWeek, hourOfDay int) (*model.BaselineProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.baselines[baselineKey(metricKey, dayOfWeek, hourOfDay)], nil
}

func (r *fakeRepo) UpdateBaselineProfile(ctx context.Context, profile *model.BaselineProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baselines[baselineKey(profile.MetricKey, profile.DayOfWeek, profile.HourOfDay)] = profile
	return nil
}

func (r *fakeRepo) Close(ctx context.Context) error { return nil }

// seedBaseline pre-populates a tight, "normal" seasonal baseline for a metric
// at the current wall-clock day/hour, far enough from the simulator's actual
// value range that the real signal it emits is guaranteed to read as a large
// seasonal z-score deviation — this is what makes the "significant" test
// deterministic rather than relying on the simulator's randomness alone.
func (r *fakeRepo) seedBaseline(now time.Time, metricKey string, mean, stdDev float64) {
	_ = r.UpdateBaselineProfile(context.Background(), &model.BaselineProfile{
		MetricKey:   metricKey,
		DayOfWeek:   int(now.Weekday()),
		HourOfDay:   now.Hour(),
		MeanValue:   mean,
		StdDevValue: stdDev,
		SampleCount: 10, // must be > 5 for memory.EvaluateBaseline to trust it over the raw sample
		LastUpdated: now,
	})
}

func newTestCollector(t *testing.T, repo *fakeRepo) *pipeline.Collector {
	t.Helper()
	ctx := context.Background()

	memoryEngine := memory.NewMemoryEngine(repo)
	aiService, err := ai.NewAIService(ctx, &config.Config{}) // no Gemini key -> deterministic fallback synthesis
	if err != nil {
		t.Fatalf("failed to construct fallback AI service: %v", err)
	}
	corrEngine := correlation.NewCorrelationEngine(aiService, memoryEngine)
	sim := provider.NewSimulatorProvider(&config.Config{CountryCode: "LK"})

	return pipeline.NewCollector(repo, corrEngine, noopPublisher{}, sim)
}

type noopPublisher struct{}

func (noopPublisher) PublishBusinessEvent(ctx context.Context, event *model.BusinessEvent) error {
	return nil
}
func (noopPublisher) Close() error { return nil }

// TestRunOneShot_ColdStart_SavesSignalsButGeneratesNoEvent verifies the
// significance gate's expected cold-start behaviour: with no baseline
// history at all, every seasonal z-score is 0 (memory.go has nothing to
// compare against yet), so nothing should be flagged significant and no
// BusinessEvent should be produced — the pipeline should not hallucinate an
// event out of the very first batch it ever sees.
func TestRunOneShot_ColdStart_SavesSignalsButGeneratesNoEvent(t *testing.T) {
	repo := newFakeRepo()
	collector := newTestCollector(t, repo)

	if err := collector.RunOneShot(context.Background()); err != nil {
		t.Fatalf("RunOneShot failed: %v", err)
	}

	repo.mu.Lock()
	signalCount := len(repo.signals)
	eventCount := len(repo.events)
	repo.mu.Unlock()

	if signalCount == 0 {
		t.Fatalf("expected the simulator's signals to be persisted even with no event, got 0")
	}
	if eventCount != 0 {
		t.Fatalf("expected 0 BusinessEvents on a cold start with no baseline history, got %d", eventCount)
	}
}

// TestRunOneShot_SeededAnomalousBaseline_GeneratesAndPersistsEvent is the
// positive-path counterpart: once a seasonal baseline exists and the current
// reading deviates sharply from it, the significance gate should fire, the
// AI layer (fallback synthesis, since no Gemini key is configured) should
// classify it, and the resulting event should be both persisted to the
// timeline and handed to the publisher — end to end, through main.go's
// actual construction order, not a mocked slice of it.
func TestRunOneShot_SeededAnomalousBaseline_GeneratesAndPersistsEvent(t *testing.T) {
	repo := newFakeRepo()
	now := time.Now()
	// Simulator emits "simulated_google_analytics:active_users" in [450,600).
	// A seeded "normal" baseline of 50±5 guarantees a seasonal z-score far
	// past the significance threshold regardless of the exact random draw.
	repo.seedBaseline(now, "simulated_google_analytics:active_users", 50, 5)

	collector := newTestCollector(t, repo)

	if err := collector.RunOneShot(context.Background()); err != nil {
		t.Fatalf("RunOneShot failed: %v", err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()

	if len(repo.events) != 1 {
		t.Fatalf("expected exactly 1 BusinessEvent once a metric reads far outside its seeded baseline, got %d", len(repo.events))
	}
	evt := repo.events[0]
	if evt.EventID == "" {
		t.Errorf("expected the collector to assign an EventID before persisting")
	}
	if evt.Severity == "Low" && evt.EventType == "NormalBusinessActivity" {
		t.Errorf("a seeded anomaly should not resolve to NormalBusinessActivity/Low — the publish safety-net should have caught this upstream if it did")
	}
	rules, _ := evt.Metadata["triggeredRules"].([]string)
	if len(rules) == 0 {
		t.Errorf("expected triggeredRules to be recorded in event metadata, got none")
	}
}

// TestRunOneShot_RepeatedSignificantWindow_SuppressesSecondPublish exercises
// the §02 suppression window end-to-end: the correlation engine's dedup map
// is stateful across calls, and a single Collector instance is used for
// repeated daemon batches in production, so this proves that statefulness
// survives across two real RunOneShot passes rather than only the isolated
// CorrelationEngine unit tests.
func TestRunOneShot_RepeatedSignificantWindow_SuppressesSecondPublish(t *testing.T) {
	repo := newFakeRepo()
	now := time.Now()
	repo.seedBaseline(now, "simulated_google_analytics:active_users", 50, 5)

	collector := newTestCollector(t, repo)

	if err := collector.RunOneShot(context.Background()); err != nil {
		t.Fatalf("first RunOneShot failed: %v", err)
	}
	if err := collector.RunOneShot(context.Background()); err != nil {
		t.Fatalf("second RunOneShot failed: %v", err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()

	if len(repo.events) != 1 {
		t.Fatalf("expected the second run's identical (eventType, category) to be suppressed within the window, got %d events", len(repo.events))
	}
}

// TestRunDaemon_ConcurrentIngestDuringShutdown exercises the exact race this
// review found and fixed: RunDaemon's poll producers, an external pusher
// (standing in for the webhook handler) calling Ingest concurrently, and
// context cancellation all overlapping. Success here is "no panic and a
// clean return" — a send-on-closed-channel panic would fail the whole test
// binary, not just report a failure, so this is a meaningful check even
// though it makes no further assertions.
func TestRunDaemon_ConcurrentIngestDuringShutdown(t *testing.T) {
	repo := newFakeRepo()
	collector := newTestCollector(t, repo)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Hammer Ingest concurrently with RunDaemon's own producers and its
		// eventual shutdown — this is what would surface a close-on-send
		// panic if the ingestMu guard regressed.
		for i := 0; i < 200; i++ {
			collector.Ingest(model.Signal{
				Source:    "stripe",
				Type:      "payment_succeeded",
				Value:     float64(i),
				Timestamp: time.Now(),
			})
		}
	}()

	if err := collector.RunDaemon(ctx); err != nil {
		t.Fatalf("RunDaemon returned an error: %v", err)
	}
	wg.Wait()

	repo.mu.Lock()
	signalCount := len(repo.signals)
	repo.mu.Unlock()
	if signalCount == 0 {
		t.Fatalf("expected at least some ingested signals to have been persisted before shutdown")
	}
}
