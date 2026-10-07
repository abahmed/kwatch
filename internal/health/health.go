package health

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

// HealthServer serves the liveness, readiness and health endpoints.
type HealthServer struct {
	server          *http.Server
	listener        net.Listener
	port            int
	enabled         bool
	ready           atomic.Bool
	componentMu     sync.RWMutex
	componentErrors map[string]string
	componentStatus map[string]ComponentStatus
	clock           clock.Clock
	lifecycleMu     sync.Mutex
	started         bool
	stopped         bool
	stopErr         error
	serveErr        error
	serveErrors     chan error
	leadership      LeadershipStatus
	coverage        *CoverageSummary
	statusHolder    statusHolder
}

// HealthResponse is the JSON body of the /health endpoint.
type HealthResponse struct {
	Status     string            `json:"status"`
	Leadership *LeadershipStatus `json:"leadership,omitempty"`
	// Degraded names the optional components that failed to start, with the
	// reason. Empty when everything kwatch was asked to run is running.
	Degraded   map[string]string          `json:"degraded,omitempty"`
	Components map[string]ComponentStatus `json:"components,omitempty"`
	// Coverage summarises which Kubernetes resource types are watched.
	// It is informational and never affects readiness.
	Coverage *CoverageSummary `json:"coverage,omitempty"`
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
		port:            cfg.Port,
		enabled:         cfg.Enabled,
		componentErrors: make(map[string]string),
		componentStatus: make(map[string]ComponentStatus),
		serveErrors:     make(chan error, 1),
		clock:           clockSource,
	}
	return h
}
