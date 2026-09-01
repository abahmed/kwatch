package health

import (
	"encoding/json"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/feature"
)

func (h *HealthServer) kubeletHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.telemetryLister == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(
		h.telemetryLister.TelemetryStatus(),
	); err != nil {
		klog.ErrorS(err, "health: encode kubelet telemetry status")
	}
}

func (h *HealthServer) securityHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.securityLister == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(
		h.securityLister.SecurityStatus(),
	); err != nil {
		klog.ErrorS(err, "health: encode security status")
	}
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
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(
		h.controlPlaneLister.ControlPlaneStatus(),
	); err != nil {
		klog.ErrorS(err, "health: encode control-plane status")
	}
}

func (h *HealthServer) informerHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.informerLister == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(
		h.informerLister.InformerStatus(),
	); err != nil {
		klog.ErrorS(err, "health: encode informer status")
	}
}

func (h *HealthServer) featuresHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := struct {
		PolicySource string             `json:"policySource"`
		Tier         string             `json:"tier"`
		ExpiresAt    time.Time          `json:"expiresAt,omitempty"`
		Features     []feature.Decision `json:"features"`
	}{
		PolicySource: h.featurePlan.PolicySource,
		Tier:         h.featurePlan.Tier,
		ExpiresAt:    h.featurePlan.ExpiresAt,
		Features:     h.featurePlan.DecisionsList(),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		klog.ErrorS(err, "health: encode feature plan")
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
	if err := json.NewEncoder(w).Encode(snap); err != nil {
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
	if h.alertManager == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("alert manager not available")); err != nil {
			klog.ErrorS(err, "health: write alertman-not-available response")
		}
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
	h.alertManager.NotifyEvent(ev)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("test alert sent")); err != nil {
		klog.ErrorS(err, "health: write test-alert-sent response")
	}
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
	if err := json.NewEncoder(w).Encode(
		h.deadLetterLister.DeadLetters(),
	); err != nil {
		klog.ErrorS(err, "health: encode dead letters")
	}
}
