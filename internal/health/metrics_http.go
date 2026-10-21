package health

import (
	"net/http"
	"sync/atomic"

	"github.com/abahmed/kwatch/internal/metrics"
)

// MetricsProText is the startup log line of an edition without
// /metrics.
const MetricsProText = "metrics endpoint is part of Pro. Pro is free " +
	"during early access; add a key in the installer's License / Pro " +
	"menu."

// metricsGate refuses /metrics while the edition does not include it.
// The zero value serves it.
type metricsGate struct {
	denied atomic.Bool
}

// DenyMetrics makes /metrics answer a plain 404: the endpoint is not
// part of the running edition.
func (h *HealthServer) DenyMetrics() { h.metricsGate.denied.Store(true) }

// AllowMetrics lifts DenyMetrics: the edition changed to one that
// includes /metrics.
func (h *HealthServer) AllowMetrics() { h.metricsGate.denied.Store(false) }

// metricsHandler serves the default registry unless the edition denies
// it.
func (h *HealthServer) metricsHandler() http.Handler {
	served := metrics.DefaultRegistry().Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.metricsGate.denied.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		served.ServeHTTP(w, r)
	})
}
