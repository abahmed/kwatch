package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
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

type HealthServer struct {
	server       *http.Server
	port         int
	enabled      bool
	pprof        bool
	incidentAPI  IncidentLister
	alertManager TestAlertSender
}

type HealthResponse struct {
	Status string `json:"status"`
}

func NewHealthServer(cfg config.HealthCheck) *HealthServer {
	return &HealthServer{
		port:    cfg.Port,
		enabled: cfg.Enabled,
		pprof:   cfg.Pprof,
	}
}

func (h *HealthServer) SetIncidentAPI(lister IncidentLister) {
	h.incidentAPI = lister
}

func (h *HealthServer) SetAlertManager(a TestAlertSender) {
	h.alertManager = a
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
	mux.HandleFunc("/incidents", h.incidentsHandler)
	mux.HandleFunc("/test-alert", h.testAlertHandler)

	if h.pprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/block", pprof.Handler("block"))
		mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	}

	h.server = &http.Server{
		Addr:         ":" + strconv.Itoa(h.port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
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

func (h *HealthServer) readyzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *HealthServer) incidentsHandler(w http.ResponseWriter, r *http.Request) {
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
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "TestAlert",
		Events:    "this is a test alert from kwatch",
	}
	h.alertManager.NotifyEvent(ev)
	h.alertManager.Notify("[test-alert] kwatch test alert sent")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("test alert sent"))
}
