package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/client-go/tools/leaderelection"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/metrics"
)

// The callbacks leader election runs when leadership starts, ends or moves.

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
	// renewalNanos is the newest Lease write, kept so the "leader" status
	// starts with the acquire write that came before the callback.
	renewalNanos atomic.Int64
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
		LastRenewal:   c.lastRenewal(),
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

// onNewLeader reports who holds the Lease. client-go runs it in its own
// goroutine, so it can arrive after this pod became leader or stopped. It
// holds c.mu while it checks and writes, so it can never overwrite the
// "leader" or "stopped" status that markStarted and onStoppedLeading
// protect.
func (c *leaderCallbacks) onNewLeader(newLeader string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started.Load() || c.stopped {
		return
	}
	if newLeader != "" && newLeader != c.identity {
		c.observedLeader.Store(true)
	}
	c.deps.healthServer.SetLeadership(health.LeadershipStatus{
		Role: "starting", Identity: newLeader,
	})
}

func (c *leaderCallbacks) lastRenewal() time.Time {
	if nanos := c.renewalNanos.Load(); nanos != 0 {
		return time.Unix(0, nanos)
	}
	return time.Time{}
}

func (c *leaderCallbacks) recordRenewal(renewal time.Time) {
	c.renewalNanos.Store(renewal.UnixNano())
	c.deps.healthServer.SetLeadershipRenewal(renewal)
}
