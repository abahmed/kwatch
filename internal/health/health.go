package health

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

type IncidentLister interface {
	Snapshot() []model.IncidentView
}

type AlertSender interface {
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

// Dependencies are configured once before Open. Keeping this boundary typed
// makes health wiring visible at the application composition root.
type Dependencies struct {
	Incident          IncidentLister
	Delivery          AlertSender
	DeadLetters       DeadLetterLister
	Telemetry         StatusProvider
	AdoptionTelemetry StatusProvider
	Security          StatusProvider
	ControlPlane      StatusProvider
	Informer          StatusProvider
	Persistence       StatusProvider
}

type HealthServer struct {
	server                  *http.Server
	listener                net.Listener
	port                    int
	enabled                 bool
	pprof                   bool
	diagnostics             bool
	diagnosticsToken        string
	incidentAPI             IncidentLister
	deliveryManager         AlertSender
	deadLetterLister        DeadLetterLister
	telemetryLister         StatusProvider
	adoptionTelemetryLister StatusProvider
	securityLister          StatusProvider
	controlPlaneLister      StatusProvider
	informerLister          StatusProvider
	persistenceLister       StatusProvider
	ready                   atomic.Bool
	componentMu             sync.RWMutex
	componentErrors         map[string]string
	componentStatus         map[string]ComponentStatus
	clock                   clock.Clock
	lifecycleMu             sync.Mutex
	started                 bool
	stopped                 bool
	stopErr                 error
	serveErr                error
	serveErrors             chan error
	testAlertMu             sync.Mutex
	lastTestAlert           time.Time
	leadership              LeadershipStatus
}

type HealthResponse struct {
	Status     string            `json:"status"`
	Leadership *LeadershipStatus `json:"leadership,omitempty"`
	// Degraded names the optional components that failed to start, with the
	// reason. Empty when everything kwatch was asked to run is running.
	Degraded   map[string]string          `json:"degraded,omitempty"`
	Components map[string]ComponentStatus `json:"components,omitempty"`
}

// LeadershipStatus is the safe, bounded election state exposed by health.
// It deliberately contains no Lease object or arbitrary API error text.
type LeadershipStatus struct {
	Role           string    `json:"role"`
	Identity       string    `json:"identity,omitempty"`
	Epoch          int64     `json:"epoch,omitempty"`
	AcquiredAt     time.Time `json:"acquiredAt,omitempty"`
	LastRenewal    time.Time `json:"lastRenewal,omitempty"`
	LastTransition time.Time `json:"lastTransition,omitempty"`
	TakeoverCount  int64     `json:"takeoverCount,omitempty"`
	LossReason     string    `json:"lossReason,omitempty"`
}

// ComponentStatus is the safe diagnostic state for one runtime component.
// Details remain in logs; this type intentionally contains only bounded data.
type ComponentStatus struct {
	State          string    `json:"state"`
	Available      bool      `json:"available"`
	Reason         string    `json:"reason,omitempty"`
	LastTransition time.Time `json:"lastTransition,omitempty"`
}

// NewHealthServerWithClock constructs health state with an explicit clock so
// transition timestamps remain deterministic in tests and embedded callers.
func NewHealthServerWithClock(
	cfg config.HealthCheck,
	clockSource clock.Clock,
) *HealthServer {
	clockSource = clock.Require(clockSource)
	h := &HealthServer{
		port:             cfg.Port,
		enabled:          cfg.Enabled,
		pprof:            cfg.Pprof,
		diagnostics:      cfg.Diagnostics,
		diagnosticsToken: cfg.DiagnosticsToken,
		componentErrors:  make(map[string]string),
		componentStatus:  make(map[string]ComponentStatus),
		serveErrors:      make(chan error, 1),
		clock:            clockSource,
	}
	return h
}

// ConfigureDependencies supplies all diagnostic providers before the server
// opens its listener. Dependency mutation after startup is rejected.
func (h *HealthServer) ConfigureDependencies(
	deps Dependencies,
) error {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	if h.started {
		return fmt.Errorf("health dependencies cannot change after start")
	}
	h.incidentAPI = deps.Incident
	h.deliveryManager = deps.Delivery
	h.deadLetterLister = deps.DeadLetters
	h.telemetryLister = deps.Telemetry
	h.adoptionTelemetryLister = deps.AdoptionTelemetry
	h.securityLister = deps.Security
	h.controlPlaneLister = deps.ControlPlane
	h.informerLister = deps.Informer
	h.persistenceLister = deps.Persistence
	return nil
}
