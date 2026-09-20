package security

import (
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewRuntimeWithRuntimeConfig constructs admission monitoring from the
// immutable runtime snapshot.
func NewRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink monitor.ReconciliationSink,
) *Runtime {
	return &Runtime{runtime: runtime, sink: sink}
}
