package health

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

type HealthServer struct {
	server  *http.Server
	port    int
	enabled bool
}

type HealthResponse struct {
	Status string `json:"status"`
}

func NewHealthServer(port int, enabled bool) *HealthServer {
	return &HealthServer{
		port:    port,
		enabled: enabled,
	}
}

func (h *HealthServer) Start(ctx context.Context) error {
	if !h.enabled {
		logrus.Debug("health check is disabled")
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthzHandler)
	mux.HandleFunc("/health", h.healthHandler)

	h.server = &http.Server{
		Addr:         ":" + strconv.Itoa(h.port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logrus.Infof("starting health check server on port %d", h.port)
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Errorf("health check server error: %v", err)
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
