package app

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/storage"
	"github.com/abahmed/kwatch/internal/upgrader"
)

// defaultDataDir holds the state file when KWATCH_DATA_DIR is unset. The
// deployment mounts the kwatch data volume here.
const defaultDataDir = "/var/lib/kwatch"

func dataDir() string {
	if dir := os.Getenv("KWATCH_DATA_DIR"); dir != "" {
		return dir
	}
	return defaultDataDir
}

// runActiveComponents is the leader session: it claims the state file,
// runs startup bookkeeping and supervises every active component until
// leadership or the application ends.
func runActiveComponents(ctx context.Context, deps *serverDeps) error {
	activeCtx, cancel := context.WithCancel(applicationContext(deps))
	defer cancel()
	go fenceOnLeadershipLoss(ctx, activeCtx, cancel)

	state, err := openStore(activeCtx, deps)
	if err != nil {
		deps.healthServer.SetComponentError("state", err)
		return fmt.Errorf("state: %w", err)
	}
	reportStoreReset(deps, state)
	disk := diskState{store: state, client: deps.clients.Kubernetes}
	session := newStartupManagerWithRuntime(
		disk, deps.runtime, deps.clients.Clock,
		newKubernetesRestartEvidence(
			deps.clients.Kubernetes, kubeclient.GetNamespace()),
	)
	result, err := session.Start(activeCtx)
	if err != nil {
		closeStore(state)
		deps.healthServer.SetComponentError("state", err)
		return fmt.Errorf("state: startup: %w", err)
	}
	deps.readiness.setCurrent("state", true)
	announceStartup(deps, state, session)

	threads := restoreThreads(deps, disk)
	defer deps.threadWake.detach()

	supervisor := newComponentSupervisor(deps.clients.Clock.Now)
	guard := &storeGuard{}
	startActiveComponents(activeCtx, deps, supervisor, activeResources{
		state: state, disk: disk, result: result, session: session,
		threads: threads, guard: guard,
	})
	select {
	case err = <-supervisor.errCh:
	case <-ctx.Done():
	}
	// Readiness goes first: the session is over from this moment, and the
	// shutdown work below can take many seconds.
	deps.readiness.withdraw()
	cancel()
	finishActiveSession(ctx, activeShutdown{
		supervisor: supervisor, delivery: deps.deliveryManager,
		threads: threads, session: session, state: state, guard: guard,
		failed:         err != nil,
		leadershipLost: leadershipLost(ctx, deps),
		lastRenewal:    lastRenewalFrom(deps),
		now:            deps.clients.Clock.Now,
	})
	return err
}

// announceStartup queues the jobs the last session never sent, then the
// startup message, so old work goes out before any new incident.
func announceStartup(
	deps *serverDeps, state *storage.Store, session *startupManager,
) {
	attachOutbox(deps, state)
	if msg, ok := session.StartupMessage(); ok {
		deps.deliveryManager.Notify(msg)
	}
}

// restoreThreads restores saved provider threads and wakes the saver
// when new ones appear. The caller detaches it when the session ends.
func restoreThreads(deps *serverDeps, disk diskState) *threadSaver {
	threads := newThreadSaver(deps.deliveryManager, disk)
	threads.Restore()
	deps.threadWake.attach(threads)
	return threads
}

// leadershipLost reports whether the leader context ended on its own. On
// a normal shutdown the application context ends first and the Lease is
// still held, because kwatch releases it only after shutdown finishes.
func leadershipLost(leaderCtx context.Context, deps *serverDeps) bool {
	return leaderCtx.Err() != nil && applicationContext(deps).Err() == nil
}

// activeShutdown is what the leader session releases when it ends.
type activeShutdown struct {
	supervisor *componentSupervisor
	delivery   deliveryStopper
	threads    *threadSaver
	session    sessionEnder
	state      *storage.Store
	guard      *storeGuard
	failed     bool
	// leadershipLost fences delivery: another replica may already lead,
	// so queued jobs are dead-lettered instead of sent.
	leadershipLost bool
	// lastRenewal and now cap the delivery drain at the remaining Lease
	// time; both nil keeps the fixed drain timeout.
	lastRenewal func() time.Time
	now         func() time.Time
}

// storeGuard records that a background writer outlived its component and
// may still reach the store. The zero value and nil mean no such writer.
type storeGuard struct{ abandoned atomic.Bool }

func (g *storeGuard) abandon() {
	if g != nil {
		g.abandoned.Store(true)
	}
}

func (g *storeGuard) busy() bool {
	return g != nil && g.abandoned.Load()
}

// sessionEnder records how the runtime session ended.
type sessionEnder interface {
	EndSession(ctx context.Context, reason string)
}

// finishActiveSession stops the leader session in order: components,
// delivery, the final thread save, the session end marker, then the state
// file. Delivery drains before the thread save so the thread IDs created
// by the last sends are saved too. Each
// step is bounded (see serve.go for the budget). The store is closed only
// when every writer has returned, including the pipeline's storage
// writer; otherwise it stays open until the process exits, because
// closing it under a running write is unsafe and the process is
// terminating anyway.
func finishActiveSession(parent context.Context, s activeShutdown) {
	stopped := waitForSupervisor(s.supervisor)
	if !stopDelivery(
		parent, s.delivery, s.leadershipLost, s.leaseDrainBudget(),
	) {
		// The final outbox write is still running against the store.
		s.guard.abandon()
	}
	flushed := s.threads.Flush(parent)
	s.threads.Close()
	reason := "graceful_shutdown"
	if s.failed {
		reason = "internal_failure"
	}
	ended := endSession(parent, s.session, reason, sessionEndTimeout)
	if !stopped || !flushed || !ended || s.guard.busy() {
		recordShutdownTimeout("state-store")
		return
	}
	closeStore(s.state)
}

// endSession writes the session end marker within timeout and reports
// whether the write returned in time.
func endSession(
	parent context.Context, session sessionEnder, reason string,
	timeout time.Duration,
) bool {
	ctx, cancel := context.WithTimeout(
		context.WithoutCancel(parent), timeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		session.EndSession(ctx, reason)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		recordShutdownTimeout("session-end")
		return false
	}
}

// activeResources are what the leader session hands its components.
type activeResources struct {
	state   *storage.Store
	disk    diskState
	result  startupResult
	session *startupManager
	threads *threadSaver
	// guard records whether a component may still write to the store.
	guard *storeGuard
}

func startActiveComponents(
	ctx context.Context,
	deps *serverDeps,
	supervisor *componentSupervisor,
	res activeResources,
) {
	// The pipeline's progress clock was stamped when the process booted. A
	// replica that waited as standby would otherwise look stalled the
	// moment it starts leading.
	deps.pipelineProgress.Touch(deps.clients.Clock.Now())
	supervisor.startOwned(ctx, componentSpec{
		name:     "delivery",
		required: true,
		progress: deliveryProgress(deps),
		run: func(ctx context.Context) error {
			return runDelivery(ctx, deps)
		},
	})
	supervisor.startOwned(ctx, componentSpec{
		name:     "pipeline",
		required: true,
		progress: deps.pipelineProgress,
		run: func(ctx context.Context) error {
			return runPipeline(ctx, deps, res.state, res.guard)
		},
	})
	upgraderConfig := deps.runtime.Lifecycle().Upgrader()
	optional := []componentSpec{
		monitoredRun(deps, "telemetry", configureTelemetryRunner(
			deps.runtime.Lifecycle().Telemetry(), res.disk,
			res.result.ClusterID, res.result.CurrentVersion,
			deps.clients.Clock.Now, deps.clients.HTTP,
		)),
		monitoredRun(deps, "upgrader", upgrader.NewUpgrader(
			&upgraderConfig, deps.deliveryManager, res.disk,
			deps.clients.HTTP,
		).CheckUpdates),
		selfReportedRun(deps, "rbac", deps.securityMonitor.Start),
		monitoredComponent(deps, "heartbeat", runHeartbeat),
		monitoredRun(deps, "alive", aliveRecorder(res.session)),
		monitoredRun(deps, "threads", runThreadSaver(res.threads)),
		monitoredRun(deps, "state-compactor",
			runCompactor(res.state, storeMetrics(deps))),
	}
	if deps.runtime.Lifecycle().CRDEnabled() {
		optional = append(optional,
			monitoredComponent(deps, "crd-watcher", runCRDWatcher))
	}
	for _, component := range optional {
		supervisor.startOptional(ctx, component)
	}
}
