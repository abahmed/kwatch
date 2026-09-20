package network

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewRuntimeWithRuntimeConfig constructs Network monitoring from the
// immutable runtime snapshot.
func NewRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink monitor.ReconciliationSink,
	now func() time.Time,
) *Runtime {
	return &Runtime{
		runtime:   runtime,
		sink:      sink,
		now:       now,
		firstSeen: make(map[string]time.Time),
	}
}
