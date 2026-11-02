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
	"github.com/abahmed/kwatch/internal/inventory"
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

// deliveryTrailSource returns what became of an incident's messages,
// oldest first.
type deliveryTrailSource func(id string) []DeliveryAttempt

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
	// ChangeHistory shows what people changed around an incident on its
	// page: the change analysis (analysis.changes) is Pro.
	ChangeHistory bool
	Scope         Scope
	Self          Self
}

// sources holds what the running pipeline provides. The listener serves
// before the pipeline is up and after it stops, so every source may be
// missing; handlers answer 503 then. The zero value has none.
type sources struct {
	incidents atomic.Pointer[incidentReader]
	status    atomic.Pointer[statusSource]
	health    atomic.Pointer[deliveryHealthSource]
	// model is the live inventory the incident page reads containers,
	// usage and changes from; trail is the delivery trail.
	model atomic.Pointer[inventory.HistoryReader]
	trail atomic.Pointer[deliveryTrailSource]
	// coverage is what kwatch watches (coverage.go).
	coverage atomic.Pointer[coverageSource]
	// usage is the kubelet usage history (detail_usage.go).
	usage atomic.Pointer[usageSource]
	// self, failed and delivery are kwatch's view of itself, its failed
	// sends and what the pagers and channels hold (self_status.go).
	self     atomic.Pointer[selfSource]
	failed   atomic.Pointer[failedSource]
	delivery atomic.Pointer[deliveryDetailSource]
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
	texts       textCache
	schemes     schemeLog
	live        *liveHub
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
		live:   newLiveHub(),
	}
}

func newServerAuth(opts Options, now clock.Clock) *Auth {
	a := NewAuth(opts.Token, opts.Password, opts.Shared, now)
	a.trustProxy = opts.TrustProxy
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

// SetModel sets the inventory the incident page reads; nil removes it,
// and the page leaves out what only the inventory knows.
func (s *Server) SetModel(m inventory.HistoryReader) {
	if m == nil {
		s.sources.model.Store(nil)
		return
	}
	s.sources.model.Store(&m)
}

// SetDeliveryTrail sets the delivery trail source; nil leaves "who was
// told" out of the incident page.
func (s *Server) SetDeliveryTrail(src func(id string) []DeliveryAttempt) {
	if src == nil {
		s.sources.trail.Store(nil)
		return
	}
	v := deliveryTrailSource(src)
	s.sources.trail.Store(&v)
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
	// Shutdown waits for idle connections; live streams never idle.
	s.server.RegisterOnShutdown(s.live.close)
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
// sign-in, sign-out and the web UI do not. Signed-in requests record
// the scheme their host was reached with (scheme.go).
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("/api/incidents", s.incidentsHandler)
	api.HandleFunc("/api/incidents/", s.incidentDetailHandler)
	api.HandleFunc("/api/status", s.statusHandler)
	api.HandleFunc("/api/trends", s.trendsHandler)
	api.HandleFunc("/api/activity", s.activityHandler)
	api.HandleFunc("/api/coverage", s.coverageHandler)
	api.HandleFunc("/api/insights", s.insightsHandler)
	api.HandleFunc("/api/events", s.eventsHandler)
	api.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux := http.NewServeMux()
	mux.Handle("/api/", s.auth.Middleware(s.recordScheme(api)))
	mux.Handle("/api/session", s.auth.requireHTTPS(
		s.auth.Middleware(s.recordScheme(
			http.HandlerFunc(sessionHandler)))))
	mux.HandleFunc("/login", s.auth.loginHandler)
	mux.HandleFunc("/logout", s.auth.logoutHandler)
	mux.Handle("/", s.assets)
	return securityHeaders(accessLog(mux), s.sendHSTS)
}

// sendHSTS reports whether a response carries Strict-Transport-Security:
// in shared mode, over https.
func (s *Server) sendHSTS(r *http.Request) bool {
	return s.opts.Shared && s.auth.isHTTPS(r)
}
