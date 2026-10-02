package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/kubeclient"
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
	// mu orders onStartedLeading against onStoppedLeading. client-go runs
	// onStartedLeading in a goroutine and onStoppedLeading as Run returns,
	// so a session could otherwise start after Run returned and nobody
	// would wait for it. Once stopped is set no session starts.
	mu             sync.Mutex
	stopped        bool
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
	if !c.markStarted() {
		return
	}
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

// markStarted records that the session starts, unless election already
// stopped, in which case it reports false and the session never runs.
func (c *leaderCallbacks) markStarted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return false
	}
	c.started.Store(true)
	return true
}

func (c *leaderCallbacks) onStoppedLeading() {
	c.mu.Lock()
	c.stopped = true
	started := c.started.Load()
	c.mu.Unlock()
	if !started {
		return
	}
	loss := c.parent.Err() == nil
	if loss {
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
	// The session may still be draining delivery when this runs; it caps
	// the drain by the last renewal, so the stopped status keeps it.
	var lastRenewal time.Time
	if status := c.deps.healthServer.LeadershipStatus(); status != nil {
		lastRenewal = status.LastRenewal
	}
	c.deps.healthServer.SetLeadership(health.LeadershipStatus{
		Role: "stopped", Identity: c.identity, LossReason: reason,
		LastRenewal: lastRenewal,
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
		Role: "starting", Identity: newLeader,
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

// validateElectionInputs rejects missing dependencies and an unsafe Lease
// name before any election starts.
func validateElectionInputs(
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
	return nil
}

// newElectionConfig is the client-go election configuration for kwatch.
func newElectionConfig(
	lock *renewalTrackingLock, callbacks *leaderCallbacks,
) leaderelection.LeaderElectionConfig {
	return leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: leaderLeaseDuration,
		RenewDeadline: leaderRenewDeadline,
		RetryPeriod:   leaderRetryPeriod,
		WatchDog:      nil,
		Callbacks:     callbacks.callbacks(),
		// client-go would release as soon as the context is cancelled, while
		// the active session is still draining delivery and writing state.
		// kwatch releases explicitly once shutdown has finished instead.
		ReleaseOnCancel: false,
		Name:            "kwatch",
	}
}

func runLeaderElectionWithRunner(
	ctx context.Context,
	deps *serverDeps,
	factory electionFactory,
	activeRunner activeComponentRunner,
) error {
	err := validateElectionInputs(deps, factory, activeRunner)
	if err != nil {
		return err
	}
	identity, err := podIdentity()
	if err != nil {
		return err
	}
	deps.healthServer.SetLeadership(health.LeadershipStatus{
		Role: "starting",
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
		delegate:  newLeaseLock(electionClient(deps.clients), identity),
		onRenewal: callbacks.recordRenewal,
	}

	elector, err := factory(newElectionConfig(lock, callbacks))
	if err != nil {
		return fmt.Errorf("create leader elector: %w", err)
	}

	elector.Run(electionCtx)
	return finishElection(ctx, deps, callbacks, lock, identity)
}

// finishElection runs after the elector returns. It waits for a started
// active session, arranges the Lease release for a clean shutdown, and
// reports why the election ended.
func finishElection(
	ctx context.Context,
	deps *serverDeps,
	callbacks *leaderCallbacks,
	lock *renewalTrackingLock,
	identity string,
) error {
	if callbacks.started.Load() {
		if err := waitForActiveSession(callbacks.activeDone); err != nil {
			return err
		}
		if ctx.Err() != nil {
			deps.setLeaseRelease(func(releaseCtx context.Context) {
				releaseLease(
					releaseCtx, lock, identity, deps.clients.Clock.Now,
				)
			})
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

// waitForSupervisor waits for the leader components to return and reports
// whether they all did within supervisorShutdownTimeout.
func waitForSupervisor(supervisor *componentSupervisor) bool {
	done := make(chan struct{})
	go func() {
		supervisor.wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(supervisorShutdownTimeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		recordShutdownTimeout("leader-components")
		return false
	}
}

// deliveryProgress returns the delivery manager as a progress source, or
// nil when there are no providers and so nothing can report progress.
func deliveryProgress(deps *serverDeps) progressReporter {
	if deps == nil || deps.deliveryManager == nil ||
		!deps.deliveryManager.HasProviders() {
		return nil
	}
	return deps.deliveryManager
}

func newKubernetesElection(
	config leaderelection.LeaderElectionConfig,
) (electionRunner, error) {
	return leaderelection.NewLeaderElector(config)
}

// activeSessionWait is how long the election waits for the session.
const activeSessionWait = activeSessionTimeout

// waitForActiveSession waits for the active leader session to finish,
// within activeSessionTimeout: the session stops its components, drains
// delivery, flushes threads and records its end, so it needs the whole
// nested budget, not one component's.
func waitForActiveSession(done <-chan struct{}) error {
	timer := time.NewTimer(activeSessionWait)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("active leader session did not stop")
	}
}

func applicationContext(deps *serverDeps) context.Context {
	if deps.ctx != nil {
		return deps.ctx
	}
	return context.Background()
}

// fenceOnLeadershipLoss ends the active session as soon as the Lease is
// lost. The state file's epoch claim stops any late write.
func fenceOnLeadershipLoss(
	leaderCtx, activeCtx context.Context, cancel context.CancelFunc,
) {
	select {
	case <-leaderCtx.Done():
		cancel()
	case <-activeCtx.Done():
	}
}

// electionClient prefers the dedicated Lease client and falls back to the
// shared client for test compositions that do not build one.
func electionClient(clients kubeclient.ClientSet) kubernetes.Interface {
	if clients.Election != nil {
		return clients.Election
	}
	return clients.Kubernetes
}
