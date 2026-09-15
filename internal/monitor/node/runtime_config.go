package node

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewRuntimeWithRuntimeConfig constructs Node monitoring from the immutable
// runtime snapshot.
func NewRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink monitor.ObservationSink,
	inhibition NodeInhibition,
	now func() time.Time,
) *Runtime {
	return &Runtime{
		runtime:    runtime,
		sink:       sink,
		inhibition: inhibition,
		now:        now,
		firstSeen:  make(map[string]time.Time),
	}
}
