package health

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

type IncidentLister interface {
	Snapshot() []model.IncidentView
}

type TestAlertSender interface {
	NotifyEvent(event event.Event)
	Notify(msg string)
}

type DeadLetterLister interface {
	DeadLetters() interface{}
}

type HealthServer struct {
	server           *http.Server
	port             int
	enabled          bool
	pprof            bool
	diagnostics      bool
	diagnosticsToken string
	incidentAPI      IncidentLister
	alertManager     TestAlertSender
	deadLetterLister DeadLetterLister
	ready            atomic.Bool
}

type HealthResponse struct {
	Status string `json:"status"`
}

func NewHealthServer(cfg config.HealthCheck) *HealthServer {
	h := &HealthServer{
		port:             cfg.Port,
		enabled:          cfg.Enabled,
		pprof:            cfg.Pprof,
		diagnostics:      cfg.Diagnostics,
		diagnosticsToken: cfg.DiagnosticsToken,
	}
	return h
}

func (h *HealthServer) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.requireDiagnosticsAuth(w, r) {
			return
		}
		next(w, r)
	}
}

func (h *HealthServer) requireDiagnosticsAuth(w http.ResponseWriter, r *http.Request) bool {
	if h.diagnosticsToken == "" {
		return true
	}
	token := r.Header.Get("Authorization")
	if subtle.ConstantTimeCompare([]byte(token), []byte("Bearer "+h.diagnosticsToken)) == 1 {
		return true
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("unauthorized"))
	return false
}

func (h *HealthServer) SetIncidentAPI(lister IncidentLister) {
	h.incidentAPI = lister
}

func (h *HealthServer) SetAlertManager(a TestAlertSender) {
	h.alertManager = a
}

func (h *HealthServer) SetDeadLetterLister(l DeadLetterLister) {
	h.deadLetterLister = l
}

func (h *HealthServer) Start(ctx context.Context) error {
	if !h.enabled {
		klog.V(4).InfoS("health check is disabled")
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthzHandler)
	mux.HandleFunc("/health", h.healthHandler)
	mux.HandleFunc("/readyz", h.readyzHandler)
	if h.diagnostics {
		mux.HandleFunc("/incidents", h.incidentsHandler)
		mux.HandleFunc("/test-alert", h.testAlertHandler)
		mux.HandleFunc("/deadletters", h.deadLettersHandler)
	}

	mux.Handle("/metrics", metrics.Default.Handler())

	if h.pprof {
		mux.HandleFunc("/debug/pprof/", h.guard(pprof.Index))
		mux.HandleFunc("/debug/pprof/cmdline", h.guard(pprof.Cmdline))
		mux.HandleFunc("/debug/pprof/profile", h.guard(pprof.Profile))
		mux.HandleFunc("/debug/pprof/symbol", h.guard(pprof.Symbol))
		mux.HandleFunc("/debug/pprof/trace", h.guard(pprof.Trace))
		mux.HandleFunc("/debug/pprof/heap", h.guard(pprof.Handler("heap").ServeHTTP))
		mux.HandleFunc("/debug/pprof/goroutine", h.guard(pprof.Handler("goroutine").ServeHTTP))
		mux.HandleFunc("/debug/pprof/block", h.guard(pprof.Handler("block").ServeHTTP))
		mux.HandleFunc("/debug/pprof/threadcreate", h.guard(pprof.Handler("threadcreate").ServeHTTP))
		mux.HandleFunc("/debug/pprof/mutex", h.guard(pprof.Handler("mutex").ServeHTTP))
	}

	h.server = &http.Server{
		Addr:              ":" + strconv.Itoa(h.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		klog.InfoS("starting health check server", "port", h.port)
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.ErrorS(err, "health check server error")
		}
	}()

	return nil
}

func (h *HealthServer) Stop(ctx context.Context) error {
	if h.server == nil {
		return nil
	}
	return h.server.Shutdown(ctx)
}

func (h *HealthServer) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *HealthServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
}

func (h *HealthServer) SetReady(v bool) {
	h.ready.Store(v)
}

func (h *HealthServer) readyzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	if !h.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *HealthServer) incidentsHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.incidentAPI == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("incident API not available"))
		return
	}
	snap := h.incidentAPI.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(snap)
}

func (h *HealthServer) testAlertHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("use POST"))
		return
	}
	if h.alertManager == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("alert manager not available"))
		return
	}
	ev := event.Event{
		PodName:       "test-pod",
		Namespace:     "default",
		Reason:        "TestAlert",
		Events:        "this is a test alert from kwatch",
		IncludeEvents: true,
		IncludeLogs:   true,
	}
	h.alertManager.NotifyEvent(ev)
	h.alertManager.Notify("[test-alert] kwatch test alert sent")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("test alert sent"))
}

func (h *HealthServer) deadLettersHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireDiagnosticsAuth(w, r) {
		return
	}
	if h.deadLetterLister == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("dead letter lister not available"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.deadLetterLister.DeadLetters())
}
