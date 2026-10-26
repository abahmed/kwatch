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

	"github.com/abahmed/kwatch/internal/status"
)

// incidentReader is anything that can list and look up incidents. The
// incident.Manager satisfies it.
type incidentReader interface {
	Incidents() []IncidentSnapshot
	Incident(id string) *IncidentSnapshot
}

// IncidentSnapshot is a detached copy of incident data safe to hand to
// API writers. It mirrors the fields of incident.Incident that the API
// exposes.
type IncidentSnapshot struct {
	ID                string
	Root              string
	Previous          string
	CauseText         string
	Tier              string
	State             string
	Opened            time.Time
	Announced         time.Time
	RecoveringSince   time.Time
	Resolved          time.Time
	Cycles            []time.Time
	Occurrences       []time.Time
	Revision          int
	Digest            string
	Timeline          []TimelineEvent
	Ack               *Ack
	RepeatCount       int
	Modes             []string
	RootReasons       []string
	Checked           []string
	Considered        []string
	Unverified        []string
	Fix               Fix
	History           []Occurrence
	Subject           string
	Merged            []string
	Impact            []string
	AlertKey          string
}

// TimelineEvent is one line of the incident timeline.
type TimelineEvent struct {
	At     time.Time
	Text   string
	Entity string
}

// Ack is an acknowledgement on the incident.
type Ack struct {
	By      string    `json:"by"`
	At      time.Time `json:"at"`
	Message string    `json:"message,omitempty"`
}

// Fix is how the incident ended.
type Fix struct {
	Change   string `json:"change,omitempty"`
	Resolved bool   `json:"resolved"`
}

// Occurrence is one earlier resolved occurrence of this root.
type Occurrence struct {
	At      time.Time `json:"at"`
	Reason  string    `json:"reason"`
	Resolved bool     `json:"resolved"`
}

// statusSource builds the /api/status report.
type statusSource func() status.Report

// Server serves the dashboard API on its own listener.
type Server struct {
	server          *http.Server
	listener        net.Listener
	port            int
	enabled         bool
	edition         string
	auth            *Auth
	incidents       incidentReader
	status          statusSource
	started         bool
	stopped         bool
	stopErr         error
	lifecycleMu     sync.Mutex
	serveErrors     chan error
}

// NewServer builds a dashboard API server. When enabled is false, Open is
// a no-op. The token is the bearer token required on every /api/* call.
// edition is the product edition name reported in /api/status.
func NewServer(
	enabled bool, port int, token, edition string,
	incidents incidentReader, status statusSource,
) *Server {
	s := &Server{
		port:     port,
		enabled:  enabled,
		edition:  edition,
		auth:     NewAuth(token),
		incidents: incidents,
		status:   status,
		serveErrors: make(chan error, 1),
	}
	return s
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

// Serve runs the already-open listener. It is intended to be called by the
// application supervisor. Stop closes the listener during shutdown.
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
	mux.Handle("/api/incidents", s.auth.Middleware(http.HandlerFunc(s.incidentsHandler)))
	mux.Handle("/api/incidents/", s.auth.Middleware(http.HandlerFunc(s.incidentDetailHandler)))
	mux.Handle("/api/status", s.auth.Middleware(http.HandlerFunc(s.statusHandler)))
	return mux
}
