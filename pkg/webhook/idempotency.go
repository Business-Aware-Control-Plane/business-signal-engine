// Package webhook implements the push-ingestion path BSAL-HB-01 §05 calls
// for: Stripe (and any future webhook-driven source) is documented as
// event-driven, not polled, but the pipeline previously had no HTTP listener
// at all. This package is that listener's supporting pieces — idempotency,
// signature verification, and the Stripe event mapping — kept separate from
// pkg/pipeline so the ingestion path doesn't need to know about Collector's
// internals beyond the narrow Sink interface it accepts.
package webhook

import (
	"sync"
	"time"
)

// IdempotencyStore remembers processed webhook event IDs for a bounded TTL,
// so a redelivered webhook — Stripe retries any response that isn't 2xx —
// is acknowledged without the underlying signal being ingested twice.
type IdempotencyStore struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

// sweepThreshold caps how large the seen-map is allowed to grow before a
// lazy cleanup pass runs — avoids a background goroutine for the modest
// volume a single payments webhook realistically produces.
const sweepThreshold = 1000

func NewIdempotencyStore(ttl time.Duration) *IdempotencyStore {
	return &IdempotencyStore{seen: make(map[string]time.Time), ttl: ttl}
}

// MarkIfNew records eventID as seen and reports whether this is the first
// time it's been observed within the TTL window. false means "duplicate —
// acknowledge it but do not process it again."
func (s *IdempotencyStore) MarkIfNew(eventID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if seenAt, ok := s.seen[eventID]; ok && now.Sub(seenAt) < s.ttl {
		return false
	}
	s.seen[eventID] = now

	if len(s.seen) >= sweepThreshold {
		for id, t := range s.seen {
			if now.Sub(t) > s.ttl {
				delete(s.seen, id)
			}
		}
	}
	return true
}
