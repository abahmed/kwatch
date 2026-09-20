package security

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewTLSRuntimeWithRuntimeConfig constructs TLS monitoring from the immutable
// runtime snapshot.
func NewTLSRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink monitor.ReconciliationSink,
	now func() time.Time,
) *TLSRuntime {
	return &TLSRuntime{runtime: runtime, sink: sink, now: now}
}
