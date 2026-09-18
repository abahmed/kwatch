package health

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

const maxDiagnosticResponseBytes = 1 << 20

const testAlertMinimumInterval = time.Minute

type boundedResponseWriter struct {
	http.ResponseWriter
	remaining int
}

func (w *boundedResponseWriter) Write(payload []byte) (int, error) {
	if len(payload) > w.remaining {
		written, _ := w.ResponseWriter.Write(payload[:w.remaining])
		w.remaining = 0
		return written, io.ErrShortWrite
	}
	written, err := w.ResponseWriter.Write(payload)
	w.remaining -= written
	return written, err
}

func (h *HealthServer) kubeletHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.telemetryLister == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.writeStatus(w, h.telemetryLister, "kubelet telemetry")
}

func (h *HealthServer) securityHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.securityLister == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.writeStatus(w, h.securityLister, "security")
}

func (h *HealthServer) controlPlaneHandler(
	w http.ResponseWriter, r *http.Request,
) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.controlPlaneLister == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.writeStatus(w, h.controlPlaneLister, "control-plane")
}

func (h *HealthServer) informerHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.informerLister == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.writeStatus(w, h.informerLister, "informer")
}

func (h *HealthServer) persistenceHandler(
	w http.ResponseWriter, r *http.Request,
) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.persistenceLister == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.writeStatus(w, h.persistenceLister, "persistence")
}

func (h *HealthServer) writeStatus(
	w http.ResponseWriter,
	provider StatusProvider,
	name string,
) {
	payload, err := provider.StatusJSON()
	if err != nil {
		klog.ErrorS(err, "health: encode status", "component", name)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(payload) > maxDiagnosticResponseBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(payload); err != nil {
		klog.ErrorS(err, "health: write status", "component", name)
	}
}

func (h *HealthServer) incidentsHandler(
	w http.ResponseWriter, r *http.Request,
) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.incidentAPI == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("incident API not available")); err != nil {
			klog.ErrorS(err, "health: write incident-not-available response")
		}
		return
	}
	snap := h.incidentAPI.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	limited := &boundedResponseWriter{
		ResponseWriter: w, remaining: maxDiagnosticResponseBytes,
	}
	if err := json.NewEncoder(limited).Encode(snap); err != nil {
		klog.ErrorS(err, "health: encode incidents snapshot")
	}
}

func (h *HealthServer) testAlertHandler(
	w http.ResponseWriter, r *http.Request,
) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusMethodNotAllowed)
		if _, err := w.Write([]byte("use POST")); err != nil {
			klog.ErrorS(err, "health: write use-POST response")
		}
		return
	}
	if h.deliveryManager == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("alert manager not available")); err != nil {
			klog.ErrorS(err, "health: write alertman-not-available response")
		}
		return
	}
	if !h.allowTestAlert() {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	ev := event.Event{
		PodName:       "test-pod",
		Namespace:     "default",
		Reason:        constant.ReasonTestAlert,
		Events:        "this is a test alert from kwatch",
		IncludeEvents: true,
		IncludeLogs:   true,
	}
	h.deliveryManager.NotifyEvent(ev)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("test alert sent")); err != nil {
		klog.ErrorS(err, "health: write test-alert-sent response")
	}
}

func (h *HealthServer) allowTestAlert() bool {
	h.testAlertMu.Lock()
	defer h.testAlertMu.Unlock()
	now := h.clock.Now()
	if !h.lastTestAlert.IsZero() &&
		now.Sub(h.lastTestAlert) < testAlertMinimumInterval {
		return false
	}
	h.lastTestAlert = now
	return true
}

func (h *HealthServer) deadLettersHandler(
	w http.ResponseWriter, r *http.Request,
) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.deadLetterLister == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("dead letter lister not available")); err != nil {
			klog.ErrorS(err, "health: write deadletter-not-available response")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	limited := &boundedResponseWriter{
		ResponseWriter: w, remaining: maxDiagnosticResponseBytes,
	}
	if err := json.NewEncoder(limited).Encode(
		safeDeadLetters(h.deadLetterLister.DeadLetters()),
	); err != nil {
		klog.ErrorS(err, "health: encode dead letters")
	}
}

func safeDeadLetters(
	entries []model.DeadLetterEntry,
) []model.DeadLetterEntry {
	result := append([]model.DeadLetterEntry(nil), entries...)
	for i := range result {
		if result[i].Error != "queue_saturated" {
			result[i].Error = "delivery_failed"
		}
	}
	return result
}
