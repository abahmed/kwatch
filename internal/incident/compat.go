package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// SetAttributionSources preserves the historical setter for embedded callers.
// Production composition uses ConfigureAttributionSources.
func (e *Engine) SetAttributionSources(sources AttributionSources) {
	_ = e.ConfigureAttributionSources(sources)
}

// NewEngine preserves the historical function-clock constructor for embedded
// callers and focused tests. Production composition uses NewEngineWithClock.
func NewEngine(cfg Config) *Engine {
	return NewEngineWithClock(
		cfg,
		clock.Func(clock.From([]func() time.Time{cfg.Now})),
	)
}
