package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/leaderelection"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/metrics"
)

const (
	leaderLeaseDuration = 30 * time.Second
	leaderRenewDeadline = 20 * time.Second
	leaderRetryPeriod   = 5 * time.Second
	leaderLeaseName     = "kwatch-leader"
)

var errLeadershipLost = errors.New("leadership lost")

type electionRunner interface {
	Run(context.Context)
}

type electionFactory func(
	leaderelection.LeaderElectionConfig,
) (electionRunner, error)

type activeComponentRunner func(context.Context, *serverDeps) error

type leaderCallbacks struct {
	parent         context.Context
	deps           *serverDeps
	identity       string
	cancelElection context.CancelFunc
	activeRunner   activeComponentRunner
	activeErrors   chan error
	activeDone     chan struct{}
	started        atomic.Bool
	epoch          atomic.Int64
	takeovers      atomic.Int64
	observedLeader atomic.Bool
}

func (c *leaderCallbacks) callbacks() leaderelection.LeaderCallbacks {
	return leaderelection.LeaderCallbacks{
		OnStartedLeading: c.onStartedLeading,
		OnStoppedLeading: c.onStoppedLeading,
		OnNewLeader:      c.onNewLeader,
	}
}

func (c *leaderCallbacks) onStartedLeading(leaderCtx context.Context) {
	c.started.Store(true)
	c.deps.persistenceGate.enable()
	currentEpoch := c.epoch.Add(1)
	if c.deps.readiness != nil {
		c.deps.readiness.begin(
			currentEpoch, c.deps.deliveryManager.HasProviders(),
		)
	}
	metrics.DefaultRegistry().LeadershipAcquisitions.Add(1)
	c.deps.healthServer.SetReady(false)
	takeoverCount := c.takeovers.Load()
	if currentEpoch > 1 || c.observedLeader.Load() {
		takeoverCount = c.takeovers.Add(1)
	}
	c.deps.healthServer.SetLeadership(health.LeadershipStatus{
		Role:          "leader",
		Identity:      c.identity,
		Epoch:         currentEpoch,
		AcquiredAt:    c.deps.clients.Clock.Now(),
		TakeoverCount: takeoverCount,
	})
	if currentEpoch > 1 || c.observedLeader.Load() {
		metrics.DefaultRegistry().LeaderTakeovers.Add(1)
	}
	if runErr := c.activeRunner(leaderCtx, c.deps); runErr != nil {
		select {
		case c.activeErrors <- runErr:
		default:
		}
		c.cancelElection()
	}
	close(c.activeDone)
}

func (c *leaderCallbacks) onStoppedLeading() {
	if !c.started.Load() {
		return
	}
	loss := c.parent.Err() == nil
	if loss {
		c.deps.persistenceGate.disable()
		metrics.DefaultRegistry().LeadershipLosses.Add(1)
	}
	if c.deps.readiness != nil {
		c.deps.readiness.end(c.epoch.Load())
	}
	c.deps.healthServer.SetReady(false)
	reason := "shutdown"
	if loss {
		reason = "leadership_lost"
	}
	c.deps.healthServer.SetLeadership(health.LeadershipStatus{
		Role: "stopped", Identity: c.identity, LossReason: reason,
	})
}

func (c *leaderCallbacks) onNewLeader(newLeader string) {
	if c.started.Load() {
		return
	}
	if newLeader != "" && newLeader != c.identity {
		c.observedLeader.Store(true)
	}
	c.deps.healthServer.SetLeadership(health.LeadershipStatus{
		Role: "standby", Identity: newLeader, LossReason: "standby",
	})
}

func (c *leaderCallbacks) recordRenewal(renewal time.Time) {
	c.deps.healthServer.SetLeadershipRenewal(renewal)
}

// runLeaderElection keeps health and election alive on every Pod while the
// active monitoring components exist only inside the elected leader session.
func runLeaderElection(ctx context.Context, deps *serverDeps) error {
	return runLeaderElectionWithFactory(
		ctx, deps, newKubernetesElection,
	)
}

func runLeaderElectionWithFactory(
	ctx context.Context,
	deps *serverDeps,
	factory electionFactory,
) error {
	return runLeaderElectionWithRunner(
		ctx, deps, factory, runActiveComponents,
	)
}

func runLeaderElectionWithRunner(
	ctx context.Context,
	deps *serverDeps,
	factory electionFactory,
	activeRunner activeComponentRunner,
) error {
	if deps == nil || deps.healthServer == nil {
		return fmt.Errorf("leader election requires health server")
	}
	if deps.clients.Kubernetes == nil {
		return fmt.Errorf("leader election requires Kubernetes client")
	}
	if deps.clients.Clock == nil {
		return fmt.Errorf("leader election requires application clock")
	}
	if factory == nil || activeRunner == nil {
		return fmt.Errorf("leader election requires runtime dependencies")
	}
	if os.Getenv("POD_NAME") != "" &&
		os.Getenv("KWATCH_LEADER_ELECTION_NAME") == "" &&
		os.Getenv("KWATCH_INSTALLATION_ID") == "" {
		return fmt.Errorf(
			"leader election requires an installation-specific Lease name",
		)
	}
	problems := validation.IsDNS1123Subdomain(electionLeaseName())
	if len(problems) > 0 {
		return fmt.Errorf(
			"invalid leader election Lease name %q: %s",
			electionLeaseName(), problems[0],
		)
	}
	identity, err := podIdentity()
	if err != nil {
		return err
	}
	deps.healthServer.SetLeadership(health.LeadershipStatus{
		Role: "standby", LossReason: "standby",
	})

	electionCtx, cancelElection := context.WithCancel(ctx)
	defer cancelElection()
	callbacks := &leaderCallbacks{
		parent:         ctx,
		deps:           deps,
		identity:       identity,
		cancelElection: cancelElection,
		activeRunner:   activeRunner,
		activeErrors:   make(chan error, 1),
		activeDone:     make(chan struct{}),
	}
	lock := &renewalTrackingLock{
		delegate:  newLeaseLock(deps.clients.Kubernetes, identity),
		onRenewal: callbacks.recordRenewal,
	}

	elector, err := factory(leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: leaderLeaseDuration,
		RenewDeadline: leaderRenewDeadline,
		RetryPeriod:   leaderRetryPeriod,
		WatchDog:      nil,
		Callbacks:     callbacks.callbacks(),
		// Release after the active session has stopped so voluntary Pod
		// termination and scale-down do not make standbys wait for expiry.
		ReleaseOnCancel: true,
		Name:            "kwatch",
	})
	if err != nil {
		return fmt.Errorf("create leader elector: %w", err)
	}

	elector.Run(electionCtx)
	if callbacks.started.Load() {
		if err := waitForActiveSession(callbacks.activeDone); err != nil {
			return err
		}
	}
	select {
	case err := <-callbacks.activeErrors:
		return err
	default:
	}
	if ctx.Err() != nil {
		return nil
	}
	return errLeadershipLost
}

func runActiveComponents(ctx context.Context, deps *serverDeps) error {
	if deps.persistenceGate != nil {
		deps.persistenceGate.enable()
		defer deps.persistenceGate.disable()
	}
	activeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if deps.activate != nil {
		if err := deps.activate(activeCtx); err != nil {
			if deps.healthServer != nil {
				deps.healthServer.SetComponentError("persistence", err)
			}
			return err
		}
	}
	supervisor := newComponentSupervisor(deps.clients.Clock.Now)
	startActiveComponents(activeCtx, deps, supervisor)
	select {
	case err := <-supervisor.errCh:
		cancel()
		waitForSupervisor(supervisor)
		return err
	case <-ctx.Done():
		cancel()
		waitForSupervisor(supervisor)
		return nil
	}
}

func waitForSupervisor(supervisor *componentSupervisor) {
	done := make(chan struct{})
	go func() {
		supervisor.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(componentShutdownTimeout):
		recordShutdownTimeout("leader-components")
	}
}

func startActiveComponents(
	ctx context.Context,
	deps *serverDeps,
	supervisor *componentSupervisor,
) {
	supervisor.startOwned(ctx, componentSpec{
		name:     "delivery",
		required: true,
		progress: deliveryProgress(deps),
		onHealthy: func() {
			if deps.readiness != nil {
				deps.readiness.setCurrent("delivery", true)
			}
		},
		run: func(ctx context.Context) error {
			return runDelivery(ctx, deps)
		},
	})
	if deps.startPersistence != nil {
		deps.startPersistence(
			ctx, supervisor, deps.persistenceGate.enabled,
		)
	} else if deps.readiness != nil {
		// Tests and embedded callers may not install persistence writers.
		// They must not wait forever on a gate they deliberately omitted.
		deps.readiness.setCurrent("persistence-writers", true)
	}
	startCoreComponents(ctx, deps, supervisor)
	for _, component := range activeOptionalComponents(deps) {
		supervisor.startOptional(ctx, deps.initialized, component)
	}
}

func deliveryProgress(deps *serverDeps) progressReporter {
	if deps == nil || deps.deliveryManager == nil ||
		!deps.deliveryManager.HasProviders() {
		return nil
	}
	return deps.deliveryManager
}

func activeOptionalComponents(deps *serverDeps) []componentSpec {
	return []componentSpec{
		monitoredRun(deps, "status", deps.statusRun),
		monitoredRun(deps, "probe", deps.probeRun),
		monitoredRun(deps, "kubelet", deps.kubeletRun),
		monitoredRun(deps, "storage-graph", deps.storageRun),
		monitoredRun(deps, "network-graph", deps.networkRun),
		monitoredRun(deps, "rbac", deps.securityRun),
		monitoredRun(deps, "control-plane", deps.controlPlaneRun),
		monitoredRun(deps, "telemetry", deps.telemetryRun),
		monitoredRun(deps, "upgrader", deps.upgradeRun),
	}
}

func monitoredRun(
	deps *serverDeps,
	name string,
	run func(context.Context) error,
) componentSpec {
	if run == nil {
		return componentSpec{name: name}
	}
	progress := newComponentProgress(componentStartTime(deps))
	return componentSpec{
		name:      name,
		cleanStop: name == "upgrader",
		onError:   degrade(deps, name),
		onHealthy: recoverComponent(deps, name),
		progress:  progress,
		run: func(ctx context.Context) error {
			return runWithProgress(ctx, deps, progress, run)
		},
	}
}

func newKubernetesElection(
	config leaderelection.LeaderElectionConfig,
) (electionRunner, error) {
	return leaderelection.NewLeaderElector(config)
}

func waitForActiveSession(done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-time.After(componentShutdownTimeout):
		return fmt.Errorf("active leader session did not stop")
	}
}
