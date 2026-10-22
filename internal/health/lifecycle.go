package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"k8s.io/klog/v2"
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
	// Open has no caller context; binding a port does not block, so a
	// background context only satisfies the listener API.
	var listenConfig net.ListenConfig
	ln, err := listenConfig.Listen(
		context.Background(), "tcp", h.server.Addr)
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
	// The log line is written under the lifecycle lock and skipped once
	// Stop ran: on an in-process rebuild nobody waits for this
	// goroutine, and the next generation resets the logger as soon as
	// Stop returns.
	h.lifecycleMu.Lock()
	server := h.server
	listener := h.listener
	if server == nil || listener == nil || h.stopped {
		h.lifecycleMu.Unlock()
		return nil
	}
	klog.InfoS("starting health check server", "port", h.port)
	h.lifecycleMu.Unlock()
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
	mux.HandleFunc("/availabilityz", h.availabilityzHandler)
	if h.statusEnabled {
		mux.HandleFunc("/status", h.statusHandler)
	}
	mux.Handle("/metrics", h.metricsHandler())
	return mux
}

// Stop shuts the HTTP server down within ctx.
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
	server, listener := h.server, h.listener
	h.lifecycleMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	err := server.Shutdown(ctx)
	// Shutdown closes only a listener that Serve already tracks. Closing
	// it here frees the port even when Serve never ran, which an
	// in-process rebuild needs before the next generation binds it.
	_ = listener.Close()
	h.lifecycleMu.Lock()
	h.stopErr = err
	h.lifecycleMu.Unlock()
	return err
}

// ServeError returns the error that ended Serve, if any.
func (h *HealthServer) ServeError() error {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	return h.serveErr
}

// ServeErrors delivers the error that ended Serve.
func (h *HealthServer) ServeErrors() <-chan error { return h.serveErrors }
