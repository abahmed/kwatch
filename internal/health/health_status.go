package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

func (h *HealthServer) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		klog.ErrorS(err, "health: write healthz response")
	}
}

func (h *HealthServer) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := HealthResponse{
		Status:     "ok",
		Components: h.ComponentStatuses(),
	}
	if degraded := h.ComponentErrors(); len(degraded) > 0 {
		response.Status = "degraded"
		response.Degraded = degraded
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		klog.ErrorS(err, "health: encode health response")
	}
}

func (h *HealthServer) SetReady(value bool) { h.ready.Store(value) }

func (h *HealthServer) SetComponentError(name string, err error) {
	h.componentMu.Lock()
	defer h.componentMu.Unlock()
	if h.componentErrors == nil {
		h.componentErrors = make(map[string]string)
	}
	if err == nil {
		delete(h.componentErrors, name)
		h.setComponentStatusLocked(name, ComponentStatus{
			State: "running", Available: true,
		})
		return
	}
	reason := safeComponentReason(err)
	status := ComponentStatus{
		State: "degraded", Available: false, Reason: reason,
	}
	h.setComponentStatusLocked(name, status)
	h.componentErrors[name] = reason
}

// SetComponentStatus records a non-error diagnostic state, such as an
// optional API that is not installed. It does not make readiness fail.
func (h *HealthServer) SetComponentStatus(
	name, state, reason string,
	available bool,
) {
	h.componentMu.Lock()
	defer h.componentMu.Unlock()
	h.setComponentStatusLocked(name, ComponentStatus{
		State: state, Available: available, Reason: reason,
	})
}

func (h *HealthServer) setComponentStatusLocked(
	name string,
	status ComponentStatus,
) {
	if h.componentStatus == nil {
		h.componentStatus = make(map[string]ComponentStatus)
	}
	previous, exists := h.componentStatus[name]
	changed := !exists || previous.State != status.State ||
		previous.Available != status.Available ||
		previous.Reason != status.Reason
	if changed {
		if status.State == "degraded" {
			metrics.DefaultRegistry().ComponentDegradations.Add(1)
		}
		if h.clock != nil {
			status.LastTransition = h.clock.Now()
		}
	} else {
		status.LastTransition = previous.LastTransition
	}
	h.componentStatus[name] = status
}

// safeComponentReason converts internal failures into a bounded vocabulary.
// Detailed errors stay in logs; health responses are operator-facing status,
// not an error transport.
func safeComponentReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "cache") &&
		strings.Contains(message, "sync"):
		return "cache_sync_failed"
	case strings.Contains(message, "source") &&
		strings.Contains(message, "config"):
		return "source_not_configured"
	case strings.Contains(message, "migration"):
		return "persistence_migration_failed"
	case strings.Contains(message, "provider") &&
		strings.Contains(message, "shutdown"):
		return "provider_shutdown_timeout"
	case strings.Contains(message, "optional") &&
		strings.Contains(message, "api"):
		return "optional_api_unavailable"
	case strings.Contains(message, "stopped"):
		return "component_stopped"
	default:
		return "component_failed"
	}
}

// ComponentErrors reports optional monitors that failed or stopped.
func (h *HealthServer) ComponentErrors() map[string]string {
	h.componentMu.RLock()
	defer h.componentMu.RUnlock()
	out := make(map[string]string, len(h.componentErrors))
	for name, message := range h.componentErrors {
		out[name] = message
	}
	return out
}

// ComponentStatuses returns safe diagnostic state for all registered
// components. The returned map is independent of server state.
func (h *HealthServer) ComponentStatuses() map[string]ComponentStatus {
	h.componentMu.RLock()
	defer h.componentMu.RUnlock()
	out := make(map[string]ComponentStatus, len(h.componentStatus))
	for name, status := range h.componentStatus {
		out[name] = status
	}
	return out
}

// readyzHandler reports whether kwatch is watching the cluster. Optional
// component failures remain degraded on /health and do not fail readiness.
func (h *HealthServer) readyzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	if !h.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("not ready")); err != nil {
			klog.ErrorS(err, "health: write not-ready response")
		}
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		klog.ErrorS(err, "health: write readyz response")
	}
}
