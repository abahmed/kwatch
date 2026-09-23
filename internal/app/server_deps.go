package app

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pvc"
)

// serverDeps contains the components owned by the application runtime. The
// grouping makes ownership and shutdown ordering visible without hiding the
// composition wiring in a generic component registry.
type serverDeps struct {
	ctx              context.Context
	cancel           context.CancelFunc
	runtime          config.RuntimeConfig
	clients          client.ClientSet
	healthServer     *health.HealthServer
	readiness        *readinessCoordinator
	deliveryManager  *delivery.Manager
	incidentEngine   *incident.Engine
	pvcMonitor       *pvc.PvcMonitor
	hbMonitor        *heartbeat.HeartbeatMonitor
	ctl              *controller.Controller
	incidentCh       chan stateSnapshot
	incidentSaver    incidentSaver
	incidentDone     <-chan struct{}
	baselineDone     <-chan struct{}
	changeDone       <-chan struct{}
	feedbackDone     <-chan struct{}
	startPersistence func(
		context.Context, *componentSupervisor, func() bool,
	)
	activate             func(context.Context) error
	persistenceGate      *persistenceGate
	initialized          <-chan struct{}
	controllerDone       chan struct{}
	controllerProgress   *componentProgress
	notifyStartup        func()
	endSession           func(context.Context, string)
	recordFailure        func(context.Context, string, string)
	notifyStartupSummary func(map[string]int)
	// recordAlive stamps the liveness marker that lets the next start report
	// how long monitoring was down.
	recordAlive     func(context.Context)
	closeAudit      func() error
	cleanup         func()
	tlsSweep        func() error
	statusRun       func(context.Context) error
	metricsRun      func(context.Context) error
	probeRun        func(context.Context) error
	kubeletRun      func(context.Context) error
	storageRun      func(context.Context) error
	networkRun      func(context.Context) error
	securityRun     func(context.Context) error
	controlPlaneRun func(context.Context) error
	telemetryRun    func(context.Context) error
	upgradeRun      func(context.Context) error
}

// componentSpec names a background component and records whether its failure
// is fatal. The application owns the goroutine and observes the returned
// error; components keep their own domain-specific stop behavior.
type componentSpec struct {
	name           string
	required       bool
	run            func(context.Context) error
	onError        func(error)
	onHealthy      func()
	progress       progressReporter
	startupTimeout time.Duration
	stallTimeout   time.Duration
	cleanStop      bool
}

// progressReporter is intentionally a lifecycle-only interface. Domain
// components update it when they synchronize, process work, or complete a
// periodic sweep; the supervisor does not need to understand their internals.
type progressReporter interface {
	LastProgress() time.Time
}

type componentProgress struct{ last atomic.Int64 }

func newComponentProgress(now time.Time) *componentProgress {
	progress := &componentProgress{}
	progress.Touch(now)
	return progress
}

func (p *componentProgress) Touch(now time.Time) {
	if p != nil {
		p.last.Store(now.UnixNano())
	}
}

func (p *componentProgress) LastProgress() time.Time {
	if p == nil {
		return time.Time{}
	}
	value := p.last.Load()
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}
