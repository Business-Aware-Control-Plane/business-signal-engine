package processor

import (
	"time"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/pkg/model"
)

type SlidingWindowAligner struct {
	WindowDuration time.Duration
}

func NewSlidingWindowAligner(duration time.Duration) *SlidingWindowAligner {
	if duration <= 0 {
		duration = 5 * time.Minute
	}
	return &SlidingWindowAligner{WindowDuration: duration}
}

// AlignToWindow groups signals falling within [now - duration, now].
func (a *SlidingWindowAligner) AlignToWindow(signals []model.Signal, now time.Time) (model.TimeWindow, []model.Signal) {
	windowStart := now.Add(-a.WindowDuration)
	window := model.TimeWindow{
		Start: windowStart,
		End:   now,
	}

	var aligned []model.Signal
	for _, s := range signals {
		if (s.Timestamp.After(windowStart) || s.Timestamp.Equal(windowStart)) && (s.Timestamp.Before(now) || s.Timestamp.Equal(now)) {
			aligned = append(aligned, s)
		}
	}

	return window, aligned
}
