package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// Open creates the HTTP listener without starting a goroutine. The application
// supervisor owns the serving goroutine and can therefore observe failures.
func (h *HealthServer) Open() error {
	if !h.enabled {
		klog.V(4).InfoS("health check is disabled")
		return nil
	}
	h.lifecycleMu.Lock()
	if h.started {
		stopped := h.stopped
		h.lifecycleMu.Unlock()
		if stopped {
			return fmt.Errorf("health check server cannot be restarted")
		}
		return nil
	}

	mux := newServeMux(h)
	h.server = &http.Server{
		Addr:              ":" + strconv.Itoa(h.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ln, err := net.Listen("tcp", h.server.Addr)
	if err != nil {
		h.lifecycleMu.Unlock()
		return err
	}
	h.listener = ln
	h.started = true
	h.lifecycleMu.Unlock()
	return nil
}

// Serve runs the already-open listener. It is intended to be called by the
// application supervisor. Stop closes the listener during shutdown.
func (h *HealthServer) Serve(_ context.Context) error {
	h.lifecycleMu.Lock()
	server := h.server
	listener := h.listener
	h.lifecycleMu.Unlock()
	if server == nil || listener == nil {
		return nil
	}
	klog.InfoS("starting health check server", "port", h.port)
	if err := server.Serve(listener); err != nil &&
		err != http.ErrServerClosed {
		h.lifecycleMu.Lock()
		h.serveErr = err
		h.lifecycleMu.Unlock()
		h.SetComponentError("health-server", err)
		select {
		case h.serveErrors <- err:
		default:
			// The health server reports at most one terminal serve error.
		}
		return err
	}
	return nil
}

func newServeMux(h *HealthServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthzHandler)
	mux.HandleFunc("/health", h.healthHandler)
	mux.HandleFunc("/readyz", h.readyzHandler)
	if h.diagnostics {
		mux.HandleFunc("/incidents", h.incidentsHandler)
		mux.HandleFunc("/test-alert", h.testAlertHandler)
		mux.HandleFunc("/deadletters", h.deadLettersHandler)
	}
	mux.HandleFunc("/kubelet", h.kubeletHandler)
	mux.HandleFunc("/security", h.securityHandler)
	mux.HandleFunc("/controlplane", h.controlPlaneHandler)
	mux.HandleFunc("/informer", h.informerHandler)
	mux.HandleFunc("/persistence", h.persistenceHandler)
	mux.Handle("/metrics", metrics.DefaultRegistry().Handler())
	if h.pprof {
		mux.HandleFunc("/debug/pprof/", h.guard(pprof.Index))
		mux.HandleFunc("/debug/pprof/cmdline", h.guard(pprof.Cmdline))
		mux.HandleFunc("/debug/pprof/profile", h.guard(pprof.Profile))
		mux.HandleFunc("/debug/pprof/symbol", h.guard(pprof.Symbol))
		mux.HandleFunc("/debug/pprof/trace", h.guard(pprof.Trace))
		mux.HandleFunc(
			"/debug/pprof/heap", h.guard(pprof.Handler("heap").ServeHTTP),
		)
		mux.HandleFunc(
			"/debug/pprof/goroutine",
			h.guard(pprof.Handler("goroutine").ServeHTTP),
		)
		mux.HandleFunc(
			"/debug/pprof/block", h.guard(pprof.Handler("block").ServeHTTP),
		)
		mux.HandleFunc(
			"/debug/pprof/threadcreate",
			h.guard(pprof.Handler("threadcreate").ServeHTTP),
		)
		mux.HandleFunc(
			"/debug/pprof/mutex", h.guard(pprof.Handler("mutex").ServeHTTP),
		)
	}
	return mux
}

func (h *HealthServer) Stop(ctx context.Context) error {
	h.lifecycleMu.Lock()
	if !h.started {
		h.lifecycleMu.Unlock()
		return nil
	}
	if h.stopped {
		err := h.stopErr
		h.lifecycleMu.Unlock()
		return err
	}
	h.stopped = true
	server := h.server
	h.lifecycleMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	err := server.Shutdown(ctx)
	h.lifecycleMu.Lock()
	h.stopErr = err
	h.lifecycleMu.Unlock()
	return err
}

func (h *HealthServer) ServeError() error {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	return h.serveErr
}

func (h *HealthServer) ServeErrors() <-chan error { return h.serveErrors }
