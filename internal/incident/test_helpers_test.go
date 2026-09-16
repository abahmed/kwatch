package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

func newTestEngine(configs ...Config) *Engine {
	cfg := Config{Window: 10 * time.Minute}
	if len(configs) > 0 {
		cfg = configs[0]
	}
	return NewEngineWithClock(cfg, clock.RealClock{})
}
