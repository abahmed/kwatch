package dashboard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/status"
)

// incidentReader lists and looks up incidents. incident.Manager satisfies it.
type incidentReader interface {
	Incidents() []incident.Incident
	Incident(id string) *incident.Incident
}

// statusSource builds the /api/status report.
type statusSource func() status.Report

// deliveryHealthSource returns per-channel delivery health.
type deliveryHealthSource func() []ChannelHealth

// Server serves the dashboard API on its own listener.
type Server struct {
	server            *http.Server
	listener          net.Listener
	port              int
	enabled           bool
	edition           string
	auth              *Auth
	incidents         incidentReader
	status            statusSource
	deliveryHealth    deliveryHealthSource
	started           bool
	stopped           bool
	stopErr           error
	lifecycleMu       sync.Mutex
	serveErrors       chan error
}

// NewServer builds a dashboard API server. When enabled is false, Open is
// a no-op. The token is the bearer token required on every /api/* call.
// edition is the product edition name reported in /api/status.
func NewServer(
	enabled bool, port int, token, edition string,
	incidents incidentReader, status statusSource,
	deliveryHealth deliveryHealthSource,
) *Server {
	s := &Server{
		port:            port,
		enabled:         enabled,
		edition:         edition,
		auth:            NewAuth(token),
		incidents:       incidents,
		status:          status,
		deliveryHealth:  deliveryHealth,
		serveErrors:     make(chan error, 1),
	}
	return s
}

// SetIncidents replaces the incident reader (called once the pipeline is up).
func (s *Server) SetIncidents(r incidentReader) {
	s.incidents = r
}

// SetStatus replaces the status source (called once the pipeline is up).
func (s *Server) SetStatus(src statusSource) {
	s.status = src
}

// SetDeliveryHealth replaces the delivery health source.
func (s *Server) SetDeliveryHealth(src deliveryHealthSource) {
	s.deliveryHealth = src
}

// Open creates the HTTP listener without starting a goroutine. The
// application supervisor owns the serving goroutine.
func (s *Server) Open() error {
	if !s.enabled {
		klog.V(4).InfoS("dashboard is disabled")
		return nil
	}
	s.lifecycleMu.Lock()
	if s.started {
		stopped := s.stopped
		s.lifecycleMu.Unlock()
		if stopped {
			return fmt.Errorf("dashboard server cannot be restarted")
		}
		return nil
	}
	mux := s.newServeMux()
	s.server = &http.Server{
		Addr:              ":" + strconv.Itoa(s.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	var listenConfig net.ListenConfig
	ln, err := listenConfig.Listen(
		context.Background(), "tcp", s.server.Addr)
	if err != nil {
		s.lifecycleMu.Unlock()
		return err
	}
	s.listener = ln
	s.started = true
	s.lifecycleMu.Unlock()
	return nil
}

// Serve runs the already-open listener.
func (s *Server) Serve(_ context.Context) error {
	s.lifecycleMu.Lock()
	server := s.server
	listener := s.listener
	if server == nil || listener == nil || s.stopped {
		s.lifecycleMu.Unlock()
		return nil
	}
	klog.InfoS("starting dashboard API server", "port", s.port)
	s.lifecycleMu.Unlock()
	if err := server.Serve(listener); err != nil &&
		err != http.ErrServerClosed {
		s.lifecycleMu.Lock()
		s.stopErr = err
		s.lifecycleMu.Unlock()
		klog.ErrorS(err, "dashboard server stopped", "component", "dashboard-server")
		select {
		case s.serveErrors <- err:
		default:
		}
		return err
	}
	return nil
}

// Stop shuts the HTTP server down within ctx.
func (s *Server) Stop(ctx context.Context) error {
	s.lifecycleMu.Lock()
	if !s.started {
		s.lifecycleMu.Unlock()
		return nil
	}
	if s.stopped {
		err := s.stopErr
		s.lifecycleMu.Unlock()
		return err
	}
	s.stopped = true
	server, listener := s.server, s.listener
	s.lifecycleMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	err := server.Shutdown(ctx)
	_ = listener.Close()
	s.lifecycleMu.Lock()
	s.stopErr = err
	s.lifecycleMu.Unlock()
	return err
}

// ServeError returns the error that ended Serve, if any.
func (s *Server) ServeError() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.stopErr
}

func (s *Server) newServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	incidents := s.auth.Middleware(http.HandlerFunc(s.incidentsHandler))
	detail := s.auth.Middleware(http.HandlerFunc(s.incidentDetailHandler))
	status := s.auth.Middleware(http.HandlerFunc(s.statusHandler))
	mux.Handle("/api/incidents", incidents)
	mux.Handle("/api/incidents/", detail)
	mux.Handle("/api/status", status)
	return mux
}
