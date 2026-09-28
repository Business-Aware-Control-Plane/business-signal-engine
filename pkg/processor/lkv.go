package processor

import (
	"strings"
	"sync"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

// defaultMaxAge is used for a source whose cadence was never registered
// (e.g. a webhook-driven source with no fixed poll frequency) so a carried
// value still degrades gracefully instead of being kept forever.
const defaultMaxAge = 30 * time.Minute

// carryDecayFactor sets how much slack beyond a source's own poll frequency
// a carried value is allowed before being dropped — 1.5x means a 15-minute
// weather poll can still be carried for up to ~22 minutes into a later window.
const carryDecayFactor = 1.5

// CarriedSignal pairs a stale-but-still-usable signal with how long ago it
// was actually observed. Age is what lets a downstream consumer (or a human
// reading the reasoning trace) tell a fresh reading from a carried one.
type CarriedSignal struct {
	Signal model.Signal
	Age    time.Duration
}

// LastKnownValueStore keeps the most recent valid signal per "source:type"
// key, independent of the sliding correlation window. It exists to close
// BSAL-HB-01 §03: a 5-minute correlation window cannot see a 15- or
// 30-minute-cadence source most of the time, so slower sources need to be
// carried forward rather than required to be fresh inside every window.
type LastKnownValueStore struct {
	mu     sync.RWMutex
	latest map[string]model.Signal
	maxAge map[string]time.Duration // keyed by Signal.Source
}

func NewLastKnownValueStore() *LastKnownValueStore {
	return &LastKnownValueStore{
		latest: make(map[string]model.Signal),
		maxAge: make(map[string]time.Duration),
	}
}

// Observe records a freshly fetched batch as the new last-known-value for
// each of its source:type keys, and registers that source's carry-forward
// ceiling from its own poll frequency. Call this once per provider fetch —
// including a successful fetch that returned zero signals — with that
// provider's own source identity and PollFrequency(), not once per merged
// batch, so each source's ceiling reflects its own cadence, not a shared
// default.
//
// A successful poll, even an empty one, is authoritative about that
// source's current state: any previously-cached key under the same source
// that this fetch didn't reconfirm is dropped immediately, rather than
// left to linger until its own age ceiling expires. Without this, a
// scheduled-event flag that genuinely ended (e.g. a campaign withdrawn, or
// simply a different scenario's fresh, campaign-free bizsim instance) could
// keep re-triggering significance for up to pollFrequency*carryDecayFactor
// after the source itself has already reported it's gone — the exact
// mechanism behind SIM-HB-01 §08's cross-scenario contamination finding
// (2026-09-28): `Observe` was silently no-op'ing on empty fetches, so an
// old campaign_window=1 reading from a prior scenario's run was never
// cleared by the next scenario's own correctly-empty polls, only by time
// alone.
func (l *LastKnownValueStore) Observe(source string, signals []model.Signal, pollFrequency time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	seenThisFetch := make(map[string]bool, len(signals))
	for _, s := range signals {
		key := s.Source + ":" + s.Type
		seenThisFetch[key] = true
		l.latest[key] = s
	}
	if pollFrequency > 0 {
		l.maxAge[source] = time.Duration(float64(pollFrequency) * carryDecayFactor)
	}

	prefix := source + ":"
	for key := range l.latest {
		if strings.HasPrefix(key, prefix) && !seenThisFetch[key] {
			delete(l.latest, key)
		}
	}
}

// CarriedForward returns every stored signal whose source:type key is not
// already present in `fresh`, still within its source's carry-forward
// ceiling, with its Confidence linearly decayed to 0 as Age approaches that
// ceiling — a stale-but-recent weather reading should count for less than a
// signal fetched moments ago, not for the same amount.
func (l *LastKnownValueStore) CarriedForward(fresh []model.Signal, now time.Time) []CarriedSignal {
	l.mu.RLock()
	defer l.mu.RUnlock()

	freshKeys := make(map[string]bool, len(fresh))
	for _, s := range fresh {
		freshKeys[s.Source+":"+s.Type] = true
	}

	var carried []CarriedSignal
	for key, s := range l.latest {
		if freshKeys[key] {
			continue
		}
		age := now.Sub(s.Timestamp)
		if age < 0 {
			continue
		}
		ceiling := defaultMaxAge
		if ma, ok := l.maxAge[s.Source]; ok && ma > 0 {
			ceiling = ma
		}
		if age > ceiling {
			continue // stale beyond this source's own cadence — drop, don't carry
		}

		decayed := s
		factor := 1.0 - (float64(age) / float64(ceiling))
		if factor < 0 {
			factor = 0
		}
		decayed.Confidence = s.Confidence * factor
		if decayed.Metadata == nil {
			decayed.Metadata = map[string]interface{}{}
		} else {
			cp := make(map[string]interface{}, len(s.Metadata)+2)
			for k, v := range s.Metadata {
				cp[k] = v
			}
			decayed.Metadata = cp
		}
		decayed.Metadata["carriedForward"] = true
		decayed.Metadata["carriedAgeSeconds"] = age.Seconds()

		carried = append(carried, CarriedSignal{Signal: decayed, Age: age})
	}
	return carried
}

// BuildContext returns fresh ∪ carried-forward-as-signals, ready to hand to
// the significance engine and correlation layer in place of the fresh window
// alone.
func BuildContext(fresh []model.Signal, lkv *LastKnownValueStore, now time.Time) []model.Signal {
	carried := lkv.CarriedForward(fresh, now)
	if len(carried) == 0 {
		return fresh
	}
	ctx := make([]model.Signal, 0, len(fresh)+len(carried))
	ctx = append(ctx, fresh...)
	for _, c := range carried {
		ctx = append(ctx, c.Signal)
	}
	return ctx
}
