package webhook_test

import (
	"testing"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/webhook"
)

func TestIdempotencyStore_FirstDeliveryIsNew(t *testing.T) {
	s := webhook.NewIdempotencyStore(time.Hour)
	if !s.MarkIfNew("evt_1") {
		t.Fatalf("expected the first delivery of evt_1 to be reported as new")
	}
}

func TestIdempotencyStore_RedeliveryWithinTTLIsDuplicate(t *testing.T) {
	s := webhook.NewIdempotencyStore(time.Hour)
	s.MarkIfNew("evt_1")
	if s.MarkIfNew("evt_1") {
		t.Fatalf("expected a redelivery within the TTL window to be reported as a duplicate")
	}
}

func TestIdempotencyStore_DistinctEventsAreIndependent(t *testing.T) {
	s := webhook.NewIdempotencyStore(time.Hour)
	if !s.MarkIfNew("evt_1") || !s.MarkIfNew("evt_2") {
		t.Fatalf("expected two distinct event IDs to both be reported as new")
	}
}
