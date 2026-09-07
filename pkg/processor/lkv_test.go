package processor_test

import (
	"testing"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/processor"
)

func TestLastKnownValueStore_CarriesForwardWithinCeiling(t *testing.T) {
	lkv := processor.NewLastKnownValueStore()
	now := time.Now()

	weatherSignal := model.Signal{Source: "weather", Type: "rain_mm", Value: 12, Confidence: 0.95, Timestamp: now.Add(-8 * time.Minute)}
	lkv.Observe([]model.Signal{weatherSignal}, 15*time.Minute) // ceiling = 22.5 min

	// A fast-cadence window 8 minutes later has no fresh weather reading.
	fresh := []model.Signal{{Source: "meta_business", Type: "ad_ctr_pct", Value: 4.1, Timestamp: now}}
	ctx := processor.BuildContext(fresh, lkv, now)

	if len(ctx) != 2 {
		t.Fatalf("expected fresh signal + 1 carried-forward weather signal, got %d", len(ctx))
	}

	var carriedRain *model.Signal
	for i := range ctx {
		if ctx[i].Source == "weather" {
			carriedRain = &ctx[i]
		}
	}
	if carriedRain == nil {
		t.Fatalf("expected carried-forward weather signal in context")
	}
	if carriedRain.Confidence >= weatherSignal.Confidence {
		t.Fatalf("expected carried confidence to decay below original 0.95, got %.3f", carriedRain.Confidence)
	}
	if v, ok := carriedRain.Metadata["carriedForward"].(bool); !ok || !v {
		t.Fatalf("expected carriedForward=true in metadata, got %v", carriedRain.Metadata["carriedForward"])
	}
}

func TestLastKnownValueStore_DropsBeyondCeiling(t *testing.T) {
	lkv := processor.NewLastKnownValueStore()
	now := time.Now()

	stale := model.Signal{Source: "calendar", Type: "public_holiday", Value: 1, Confidence: 1.0, Timestamp: now.Add(-40 * time.Minute)}
	lkv.Observe([]model.Signal{stale}, 30*time.Minute) // ceiling = 45 min, so 40m old is still carried

	ctx := processor.BuildContext(nil, lkv, now)
	if len(ctx) != 1 {
		t.Fatalf("expected 1 carried signal within 45min ceiling, got %d", len(ctx))
	}

	// Now push well past the ceiling.
	tooOld := now.Add(50 * time.Minute)
	ctxAfter := processor.BuildContext(nil, lkv, tooOld)
	if len(ctxAfter) != 0 {
		t.Fatalf("expected stale signal beyond ceiling to be dropped, got %d entries", len(ctxAfter))
	}
}

func TestLastKnownValueStore_DoesNotDuplicateFreshSignal(t *testing.T) {
	lkv := processor.NewLastKnownValueStore()
	now := time.Now()

	lkv.Observe([]model.Signal{{Source: "weather", Type: "rain_mm", Value: 5, Timestamp: now.Add(-2 * time.Minute)}}, 15*time.Minute)

	fresh := []model.Signal{{Source: "weather", Type: "rain_mm", Value: 6, Timestamp: now}}
	ctx := processor.BuildContext(fresh, lkv, now)

	if len(ctx) != 1 {
		t.Fatalf("expected only the fresh signal (no stale duplicate for the same source:type), got %d", len(ctx))
	}
}
