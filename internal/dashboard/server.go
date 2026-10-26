package dashboard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/status"
)

// incidentReader lists and looks up incidents. incident.Manager satisfies
// it.
type incidentReader interface {
	Incidents() []incident.Incident
	Incident(id string) *incident.Incident
}

// statusSource builds the /api/status report.
type statusSource func() status.Report

// deliveryHealthSource returns per-channel delivery health.
type deliveryHealthSource func() []ChannelHealth

// Scope is the namespace scope /api/status reports. An empty Watched
// list means every namespace.
type Scope struct {
	Watched  []string
	Excluded []string
}

// sources holds what the running pipeline provides. The listener serves
// before the pipeline is up and after it stops, so every source may be
// missing; handlers answer 503 then. The zero value has none.
type sources struct {
	incidents atomic.Pointer[incidentReader]
	status    atomic.Pointer[statusSource]
	health    atomic.Pointer[deliveryHealthSource]
}

// Server serves the dashboard API on its own listener.
type Server struct {
	server      *http.Server
	listener    net.Listener
	port        int
	edition     string
	scope       Scope
	auth        *Auth
	sources     sources
	started     bool
	stopped     bool
	stopErr     error
	lifecycleMu sync.Mutex
}

// NewServer builds a dashboard API server. The token is the bearer token
// required on every call; an empty token rejects every call. edition is
// reported in /api/status, scope is the configured namespace scope and
// now is the clock of the auth rate limiter.
func NewServer(
	port int, token, edition string, scope Scope, now clock.Clock,
) *Server {
	return &Server{
		port:    port,
		edition: edition,
		scope:   scope,
		auth:    NewAuth(token, now),
	}
}

// SetIncidents sets the incident reader; nil removes it (503).
func (s *Server) SetIncidents(r incidentReader) {
	if r == nil {
		s.sources.incidents.Store(nil)
		return
	}
	s.sources.incidents.Store(&r)
}

// SetStatus sets the status source; nil removes it (503).
func (s *Server) SetStatus(src func() status.Report) {
	if src == nil {
		s.sources.status.Store(nil)
		return
	}
	v := statusSource(src)
	s.sources.status.Store(&v)
}

// SetDeliveryHealth sets the delivery health source; nil reports no
// channels.
func (s *Server) SetDeliveryHealth(src func() []ChannelHealth) {
	if src == nil {
		s.sources.health.Store(nil)
		return
	}
	v := deliveryHealthSource(src)
	s.sources.health.Store(&v)
}

// Open creates the HTTP listener without starting a goroutine. The
// application supervisor owns the serving goroutine.
func (s *Server) Open() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.started {
		if s.stopped {
			return fmt.Errorf("dashboard server cannot be restarted")
		}
		return nil
	}
	s.server = &http.Server{
		Addr:              ":" + strconv.Itoa(s.port),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	var listenConfig net.ListenConfig
	ln, err := listenConfig.Listen(
		context.Background(), "tcp", s.server.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.started = true
	return nil
}

// Serve runs the already-open listener.
func (s *Server) Serve(_ context.Context) error {
	s.lifecycleMu.Lock()
	server, listener := s.server, s.listener
	if server == nil || listener == nil || s.stopped {
		s.lifecycleMu.Unlock()
		return nil
	}
	s.lifecycleMu.Unlock()
	klog.InfoS("starting dashboard API server", "port", s.port)
	err := server.Serve(listener)
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	s.lifecycleMu.Lock()
	s.stopErr = err
	s.lifecycleMu.Unlock()
	klog.ErrorS(err, "dashboard server stopped",
		"component", "dashboard-server")
	return err
}

// Stop shuts the HTTP server down within ctx.
func (s *Server) Stop(ctx context.Context) error {
	s.lifecycleMu.Lock()
	if !s.started || s.stopped {
		err := s.stopErr
		s.lifecycleMu.Unlock()
		return err
	}
	s.stopped = true
	server, listener := s.server, s.listener
	s.lifecycleMu.Unlock()
	err := server.Shutdown(ctx)
	_ = listener.Close()
	s.lifecycleMu.Lock()
	s.stopErr = err
	s.lifecycleMu.Unlock()
	return err
}

// Handler is the whole API: security headers and the access log on every
// response, then auth, then the routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/incidents", s.incidentsHandler)
	mux.HandleFunc("/api/incidents/", s.incidentDetailHandler)
	mux.HandleFunc("/api/status", s.statusHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return securityHeaders(accessLog(s.auth.Middleware(mux)))
}
