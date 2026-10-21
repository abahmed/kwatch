package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/rbac"
	"github.com/abahmed/kwatch/internal/storage"
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
	// entitlements is what the installation may run; nil is the
	// Community edition. inactivePro
	// is the Pro configuration it does not run, for /status.
	entitlements feature.Entitlements
	inactivePro  []string
	// notice is what the edition does not run, for /status and the
	// one-time upgrade message.
	notice editionNotice
	// edition is the edition the installation runs now. The leader's
	// license manager updates it; the health server and standby replicas
	// read it. It starts as the bootstrap edition.
	edition editionHolder
	// license is the licensing wiring; nil runs without licensing.
	license *licenseSetup
	// overlay is how the startup KwatchConfig turned out.
	overlay overlayOutcome
	// editionRestart is set when the leader session ended because the
	// license changed the edition; RunWithClock then rebuilds.
	editionRestart atomic.Bool
	// rebuild paces the retries of a generation built after an edition
	// change; nil on a first start, which never retries.
	rebuild *rebuildRetry
	// openedStore is the state file the leader session opened. A rebuild
	// waits for its Done before the next generation reopens the file.
	openedStore atomic.Pointer[storage.Store]
	// openState opens the state file; nil uses storage.Open.
	openState stateOpener

	// editionLogged makes logEdition log once per generation.
	editionLogged sync.Once

	// releaseLease is set after a graceful active session so shutdown can
	// hand over the Lease once delivery has drained.
	leaseMu      sync.Mutex
	releaseLease func(context.Context)
}

func newServerDeps(
	ctx context.Context, cancel context.CancelFunc, boot *bootstrap,
) *serverDeps {
	deps := &serverDeps{
		ctx: ctx, cancel: cancel, runtime: boot.runtime,
		clients:          boot.clients,
		healthServer:     boot.healthServer,
		readiness:        newReadinessCoordinator(boot.healthServer),
		deliveryManager:  boot.deliveryManager,
		securityMonitor:  boot.securityMonitor,
		heartbeat:        boot.heartbeat,
		pipelineProgress: newComponentProgress(boot.clock.Now()),
		threadWake:       boot.threadWake,
		entitlements:     boot.entitlements,
		inactivePro:      boot.inactivePro,
		notice:           boot.notice,
		license:          boot.license,
		overlay:          boot.overlay,
	}
	deps.edition.Store(boot.entitlements)
	return deps
}

// setEdition records the edition the license entitles and applies what
// depends on it before components exist: the /status JSON format.
func (d *serverDeps) setEdition(e feature.Entitlements) {
	d.edition.Store(e)
	if d.healthServer == nil {
		return
	}
	if feature.OrCommunity(e).Limits().StatusJSON {
		d.healthServer.AllowStatusJSON()
		return
	}
	d.healthServer.DenyStatusJSON()
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

// componentProgress remembers when a component last reported progress. It
// keeps the time exactly as given, monotonic reading included: the
// supervisor subtracts it from the clock's now, and a wall-clock step (an
// NTP correction, a resumed VM) must neither fake nor hide a stall.
type componentProgress struct {
	mu   sync.Mutex
	last time.Time
}

func newComponentProgress(now time.Time) *componentProgress {
	progress := &componentProgress{}
	progress.Touch(now)
	return progress
}

func (p *componentProgress) Touch(now time.Time) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.last = now
	p.mu.Unlock()
}

func (p *componentProgress) LastProgress() time.Time {
	if p == nil {
		return time.Time{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
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

// logEdition logs what the edition does not run: the Pro settings the
// configuration sets and the Pro detectors. With a license key the edition
// is not known until the leader settled the license, and a Pro start would
// otherwise warn for a second about an edition it is about to leave, so
// the leader session calls it once the edition is resolved. Without a key
// the bootstrap edition is final and the generation logs it at once.
func (d *serverDeps) logEdition() {
	d.editionLogged.Do(func() {
		logInactivePro(d.entitlements, d.inactivePro)
		logEditionNotice(d.entitlements, d.notice)
		logDroppedPagers(d.notice)
	})
}

// licenseKeyConfigured reports a configured license key. Until the leader
// settled it the edition may still change.
func (d *serverDeps) licenseKeyConfigured() bool {
	return d.license != nil && d.license.key != ""
}

// has reports whether the installation may use id.
func (d *serverDeps) has(id feature.ID) bool {
	return entitled(d.entitlements, id)
}
