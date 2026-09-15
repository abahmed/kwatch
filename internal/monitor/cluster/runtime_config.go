package cluster

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewRuntimeWithRuntimeConfig constructs cluster-resource monitoring from the
// immutable runtime snapshot.
func NewRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink monitor.ReconciliationSink,
	now func() time.Time,
) *Runtime {
	return &Runtime{runtime: runtime, sink: sink, now: now}
}

// NewEventRuntimeWithRuntimeConfig constructs Event monitoring from the
// immutable runtime snapshot.
func NewEventRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink monitor.ObservationSink,
	now func() time.Time,
) *EventRuntime {
	return &EventRuntime{
		runtime:   runtime,
		sink:      sink,
		now:       now,
		caBlocked: make(map[string]time.Time),
	}
}
