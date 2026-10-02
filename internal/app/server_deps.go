package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/rbac"
)

// serverDeps contains the components owned by the application runtime. The
// grouping makes ownership and shutdown ordering visible without hiding the
// composition wiring in a generic component registry.
type serverDeps struct {
	ctx              context.Context
	cancel           context.CancelFunc
	runtime          config.RuntimeConfig
	clients          kubeclient.ClientSet
	healthServer     *health.HealthServer
	readiness        *readinessCoordinator
	deliveryManager  *delivery.Manager
	securityMonitor  *rbac.Monitor
	heartbeat        *heartbeat.HeartbeatMonitor
	pipelineProgress *componentProgress
	threadWake       *threadWake

	// releaseLease is set after a graceful active session so shutdown can
	// hand over the Lease once delivery has drained.
	leaseMu      sync.Mutex
	releaseLease func(context.Context)
}

func newServerDeps(
	ctx context.Context, cancel context.CancelFunc, boot *bootstrap,
) *serverDeps {
	return &serverDeps{
		ctx: ctx, cancel: cancel, runtime: boot.runtime,
		clients:          boot.clients,
		healthServer:     boot.healthServer,
		readiness:        newReadinessCoordinator(boot.healthServer),
		deliveryManager:  boot.deliveryManager,
		securityMonitor:  boot.securityMonitor,
		heartbeat:        boot.heartbeat,
		pipelineProgress: newComponentProgress(boot.clock.Now()),
		threadWake:       boot.threadWake,
	}
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

func (d *serverDeps) setLeaseRelease(release func(context.Context)) {
	d.leaseMu.Lock()
	d.releaseLease = release
	d.leaseMu.Unlock()
}

func (d *serverDeps) leaseRelease() func(context.Context) {
	d.leaseMu.Lock()
	defer d.leaseMu.Unlock()
	return d.releaseLease
}
