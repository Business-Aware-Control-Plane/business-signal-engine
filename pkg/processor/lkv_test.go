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
	lkv.Observe("weather", []model.Signal{weatherSignal}, 15*time.Minute) // ceiling = 22.5 min

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
	lkv.Observe("calendar", []model.Signal{stale}, 30*time.Minute) // ceiling = 45 min, so 40m old is still carried

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

	lkv.Observe("weather", []model.Signal{{Source: "weather", Type: "rain_mm", Value: 5, Timestamp: now.Add(-2 * time.Minute)}}, 15*time.Minute)

	fresh := []model.Signal{{Source: "weather", Type: "rain_mm", Value: 6, Timestamp: now}}
	ctx := processor.BuildContext(fresh, lkv, now)

	if len(ctx) != 1 {
		t.Fatalf("expected only the fresh signal (no stale duplicate for the same source:type), got %d", len(ctx))
	}
}

// ---- Empty-fetch pruning: SIM-HB-01 §08's cross-scenario contamination
// finding (2026-09-28). A successful poll that finds nothing is
// authoritative — it should clear a stale entry immediately, not leave it
// to expire on its own ceiling.

func TestLastKnownValueStore_EmptySuccessfulFetch_ClearsStaleEntryForSameSource(t *testing.T) {
	lkv := processor.NewLastKnownValueStore()
	now := time.Now()

	campaign := model.Signal{Source: "business_calendar", Type: "campaign_window", Value: 1, Timestamp: now.Add(-1 * time.Second)}
	lkv.Observe("business_calendar", []model.Signal{campaign}, 10*time.Second) // ceiling = 15s, well within it

	// The same source polls again immediately, successfully, and finds
	// nothing — e.g. a fresh bizsim instance for the next scenario, with no
	// active campaign at all.
	lkv.Observe("business_calendar", nil, 10*time.Second)

	ctx := processor.BuildContext(nil, lkv, now)
	if len(ctx) != 0 {
		t.Fatalf("expected the stale campaign_window entry to be cleared by the next successful-but-empty poll, got %+v", ctx)
	}
}

func TestLastKnownValueStore_EmptyFetch_DoesNotClearOtherSources(t *testing.T) {
	lkv := processor.NewLastKnownValueStore()
	now := time.Now()

	weatherSignal := model.Signal{Source: "weather", Type: "rain_mm", Value: 12, Timestamp: now.Add(-1 * time.Minute)}
	lkv.Observe("weather", []model.Signal{weatherSignal}, 15*time.Minute)

	// A completely different source polls and finds nothing — must not
	// touch weather's own cached entry.
	lkv.Observe("business_calendar", nil, 10*time.Second)

	ctx := processor.BuildContext(nil, lkv, now)
	if len(ctx) != 1 || ctx[0].Source != "weather" {
		t.Fatalf("expected weather's carried entry to survive an unrelated source's empty poll, got %+v", ctx)
	}
}

func TestLastKnownValueStore_EmptyFetch_OnlyClearsKeysNotReconfirmed(t *testing.T) {
	lkv := processor.NewLastKnownValueStore()
	now := time.Now()

	lkv.Observe("meta_business", []model.Signal{
		{Source: "meta_business", Type: "ad_spend_usd", Value: 300, Timestamp: now.Add(-5 * time.Second)},
		{Source: "meta_business", Type: "ad_ctr_pct", Value: 0.3, Timestamp: now.Add(-5 * time.Second)},
	}, 5*time.Second)

	// Next poll reconfirms only one of the two keys.
	lkv.Observe("meta_business", []model.Signal{
		{Source: "meta_business", Type: "ad_spend_usd", Value: 40, Timestamp: now},
	}, 5*time.Second)

	ctx := processor.BuildContext(nil, lkv, now)
	if len(ctx) != 1 {
		t.Fatalf("expected exactly 1 cached entry left (ad_ctr_pct should have been pruned since the second poll didn't reconfirm it), got %+v", ctx)
	}
	if ctx[0].Type != "ad_spend_usd" || ctx[0].Value != 40 {
		t.Fatalf("expected the surviving entry to be ad_spend_usd at its new value 40, got %+v", ctx[0])
	}
}
