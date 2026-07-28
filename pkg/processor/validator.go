package processor

import (
	"log"
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

type SignalValidator struct {
	seenSignals map[string]time.Time
}

func NewSignalValidator() *SignalValidator {
	return &SignalValidator{
		seenSignals: make(map[string]time.Time),
	}
}

// ValidateAndClean filters out invalid timestamps, negative value errors, and duplicate signals.
func (v *SignalValidator) ValidateAndClean(signals []model.Signal) []model.Signal {
	var valid []model.Signal
	now := time.Now()

	for _, s := range signals {
		if s.Source == "" || s.Type == "" {
			log.Printf("[WARN] [Validator] Dropping signal with empty Source or Type")
			continue
		}

		if s.Timestamp.IsZero() {
			log.Printf("[WARN] [Validator] Dropping signal with zero timestamp from %s", s.Source)
			continue
		}

		// De-duplication key
		dedupKey := s.Source + ":" + s.Type + ":" + s.Timestamp.Format(time.RFC3339)
		if lastSeen, exists := v.seenSignals[dedupKey]; exists && now.Sub(lastSeen) < 1*time.Minute {
			log.Printf("[DEBUG] [Validator] Dropping duplicate signal key: %s", dedupKey)
			continue
		}
		v.seenSignals[dedupKey] = now

		valid = append(valid, s)
	}

	return valid
}
