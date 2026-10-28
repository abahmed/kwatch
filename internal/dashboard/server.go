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

// Self names kwatch's own Deployment, so the dashboard can show the
// command that reads kwatch's logs.
type Self struct {
	Namespace  string
	Deployment string
}

// Options configure a dashboard server.
type Options struct {
	Port int
	// Token is the bearer token every /api call may carry; empty refuses
	// bearer access.
	Token string
	// Password is the shared sign-in password; empty refuses sign-in.
	Password string
	// Shared is true when people reach the dashboard through an Ingress:
	// the cookie is Secure and sign-in over plain HTTP is refused.
	Shared bool
	// TrustProxy honours X-Forwarded-Proto/For from the proxy in front
	// (Ingress, Gateway, Route or an outside load balancer). Without it
	// only TLS that kwatch serves itself counts as https.
	TrustProxy bool
	// TLSCertFile and TLSKeyFile make kwatch serve TLS itself; the files
	// are read again when they change, so a renewed certificate is used
	// without a restart.
	TLSCertFile string
	TLSKeyFile  string
	Edition     string
	Scope       Scope
	Self        Self
}

// sources holds what the running pipeline provides. The listener serves
// before the pipeline is up and after it stops, so every source may be
// missing; handlers answer 503 then. The zero value has none.
type sources struct {
	incidents atomic.Pointer[incidentReader]
	status    atomic.Pointer[statusSource]
	health    atomic.Pointer[deliveryHealthSource]
	// lastScan is the Unix nanosecond time of the engine's latest loop
	// iteration; zero until the first.
	lastScan atomic.Int64
}

// Server serves the dashboard API and its web UI on its own listener.
type Server struct {
	server      *http.Server
	listener    net.Listener
	opts        Options
	now         clock.Clock
	auth        *Auth
	assets      *assets
	sources     sources
	started     bool
	stopped     bool
	stopErr     error
	lifecycleMu sync.Mutex
}

// NewServer builds a dashboard server. now is the clock of sessions, the
// auth rate limiter and the scan-health check.
func NewServer(opts Options, now clock.Clock) *Server {
	return &Server{
		opts:   opts,
		now:    now,
		auth:   newServerAuth(opts, now),
		assets: mustAssets(),
	}
}

func newServerAuth(opts Options, now clock.Clock) *Auth {
	a := NewAuth(opts.Token, opts.Password, opts.Shared, now)
	a.trustProxy = opts.TrustProxy
	if opts.TrustProxy {
		a.failRate = newRateLimiter(now, maxProxyFailures)
	}
	return a
}

// NoteScan records that the engine finished a loop iteration now.
func (s *Server) NoteScan() {
	s.sources.lastScan.Store(s.now.Now().UnixNano())
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

// Open creates the HTTP listener within ctx without starting a goroutine.
// The application supervisor owns the serving goroutine.
func (s *Server) Open(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.started {
		if s.stopped {
			return fmt.Errorf("dashboard server cannot be restarted")
		}
		return nil
	}
	s.server = &http.Server{
		Addr:              ":" + strconv.Itoa(s.opts.Port),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	var listenConfig net.ListenConfig
	ln, err := listenConfig.Listen(ctx, "tcp", s.server.Addr)
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
	klog.InfoS("starting dashboard server", "port", s.opts.Port,
		"shared", s.opts.Shared)
	err := s.serve(server, listener)
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

// Handler is the whole dashboard: security headers and the access log
// on every response; /api/* needs a session cookie or the bearer token;
// sign-in, sign-out and the web UI do not.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("/api/incidents", s.incidentsHandler)
	api.HandleFunc("/api/incidents/", s.incidentDetailHandler)
	api.HandleFunc("/api/namespaces", s.namespacesHandler)
	api.HandleFunc("/api/status", s.statusHandler)
	api.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux := http.NewServeMux()
	mux.Handle("/api/", s.auth.Middleware(api))
	mux.Handle("/api/session", s.auth.requireHTTPS(
		s.auth.Middleware(http.HandlerFunc(sessionHandler))))
	mux.HandleFunc("/login", s.auth.loginHandler)
	mux.HandleFunc("/logout", s.auth.logoutHandler)
	mux.Handle("/", s.assets)
	return securityHeaders(accessLog(mux))
}
