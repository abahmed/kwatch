package health

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

type IncidentLister interface {
	Snapshot() []model.IncidentView
}

type TestAlertSender interface {
	NotifyEvent(event event.Event)
	Notify(msg string)
}

type DeadLetterLister interface {
	DeadLetters() []model.DeadLetterEntry
}

// StatusProvider returns an already typed JSON status document. The health
// package does not need to know every monitor's private status structure, but
// it does require an explicit serialization boundary instead of interface{}
// values flowing through every handler.
type StatusProvider interface {
	StatusJSON() ([]byte, error)
}

type HealthServer struct {
	server             *http.Server
	listener           net.Listener
	port               int
	enabled            bool
	pprof              bool
	diagnostics        bool
	diagnosticsToken   string
	incidentAPI        IncidentLister
	deliveryManager    TestAlertSender
	deadLetterLister   DeadLetterLister
	telemetryLister    StatusProvider
	securityLister     StatusProvider
	controlPlaneLister StatusProvider
	informerLister     StatusProvider
	persistenceLister  StatusProvider
	ready              atomic.Bool
	componentMu        sync.RWMutex
	componentErrors    map[string]string
	componentStatus    map[string]ComponentStatus
	lifecycleMu        sync.Mutex
	started            bool
	stopped            bool
	stopErr            error
	serveErr           error
	serveErrors        chan error
}

type HealthResponse struct {
	Status string `json:"status"`
	// Degraded names the optional components that failed to start, with the
	// reason. Empty when everything kwatch was asked to run is running.
	Degraded   map[string]string          `json:"degraded,omitempty"`
	Components map[string]ComponentStatus `json:"components,omitempty"`
}

// ComponentStatus is the safe diagnostic state for one runtime component.
// Details remain in logs; this type intentionally contains only bounded data.
type ComponentStatus struct {
	State     string `json:"state"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func NewHealthServer(cfg config.HealthCheck) *HealthServer {
	h := &HealthServer{
		port:             cfg.Port,
		enabled:          cfg.Enabled,
		pprof:            cfg.Pprof,
		diagnostics:      cfg.Diagnostics,
		diagnosticsToken: cfg.DiagnosticsToken,
		componentErrors:  make(map[string]string),
		componentStatus:  make(map[string]ComponentStatus),
		serveErrors:      make(chan error, 1),
	}
	return h
}

func (h *HealthServer) SetIncidentAPI(lister IncidentLister) {
	h.incidentAPI = lister
}

func (h *HealthServer) SetDeliveryManager(sender TestAlertSender) {
	h.deliveryManager = sender
}

func (h *HealthServer) SetDeadLetterLister(l DeadLetterLister) {
	h.deadLetterLister = l
}

func (h *HealthServer) SetTelemetryLister(l StatusProvider) {
	h.telemetryLister = l
}

func (h *HealthServer) SetSecurityLister(l StatusProvider) {
	h.securityLister = l
}

func (h *HealthServer) SetControlPlaneLister(l StatusProvider) {
	h.controlPlaneLister = l
}

func (h *HealthServer) SetInformerLister(l StatusProvider) {
	h.informerLister = l
}

func (h *HealthServer) SetPersistenceLister(l StatusProvider) {
	h.persistenceLister = l
}
