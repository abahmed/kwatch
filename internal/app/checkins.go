package app

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/license"
)

// clusterCheckins is the Pro check-in to api.kwatch.dev. Its ID is outside
// the catalog, so Community never has it.
const clusterCheckins feature.ID = "license.checkins"

const (
	checkinInterval = 30 * time.Minute
	// checkinJitter spreads check-ins to checkinInterval ± checkinJitter
	// so clusters do not reach the Worker in lockstep.
	checkinJitter = 2 * time.Minute
	// checkinStartJitter bounds the random delay before the first
	// check-in, so clusters restarting together spread out.
	checkinStartJitter = time.Minute
	checkinReadyCheck  = 15 * time.Second
	checkinTimeout     = 10 * time.Second
	// checkinStopTimeout bounds the shutdown check-in well inside the
	// termination grace period.
	checkinStopTimeout = 2 * time.Second
	stateCheckins      = "license.checkins"
)

// checkinPinger is the part of the license manager check-ins use.
type checkinPinger interface {
	Status() license.Status
	Ping(context.Context, license.Ping) error
}

// checkinRecord is whether the server last heard check-ins on, so turning
// them off sends exactly one checkins:false.
type checkinRecord struct {
	On bool `json:"on"`
}

type checkinRunner struct {
	pinger      checkinPinger
	disk        diskState
	enabled     bool
	clusterName string
	namespace   string
	ready       func() bool
	wait        func(context.Context, time.Duration) bool
	// randN returns a uniform duration in [0, n); tests inject it.
	randN func(time.Duration) time.Duration
	// blocked is the license the server refused check-ins for; they
	// resume when the license changes.
	blocked string
}

// newCheckinRunner returns nil when check-ins cannot run here: no Pro
// entitlement or no license session.
func newCheckinRunner(
	deps *serverDeps, lic *licenseSession, disk diskState,
) *checkinRunner {
	if lic == nil || lic.mgr == nil || !deps.has(clusterCheckins) {
		return nil
	}
	ready := func() bool { return true }
	if deps.healthServer != nil {
		ready = deps.healthServer.Ready
	}
	return &checkinRunner{
		pinger: lic.mgr, disk: disk,
		enabled:     deps.runtime.Lifecycle().ClusterCheckins(),
		clusterName: deps.runtime.Application().ClusterName,
		namespace:   kubeclient.GetNamespace(),
		ready:       ready, wait: waitTelemetry,
		randN: rand.N[time.Duration],
	}
}

// run checks in about every 30 minutes, after a short random start
// delay, while monitoring is ready and Pro is on.
// With check-ins off it tells the server once, then idles.
func (c *checkinRunner) run(ctx context.Context) error {
	if !c.enabled {
		c.announceOff(ctx)
		<-ctx.Done()
		return nil
	}
	klog.InfoS("cluster check-ins on: every 30 minutes kwatch sends a "+
		"hashed cluster ID, the version, the node count, the namespace "+
		"and the cluster name; set clusterCheckins=false to turn off",
		"interval", checkinInterval)
	delay := c.randN(checkinStartJitter + 1)
	for c.wait(ctx, delay) {
		delay = c.tick(ctx)
	}
	return nil
}

// interval is checkinInterval with uniform jitter of ± checkinJitter.
func (c *checkinRunner) interval() time.Duration {
	return checkinInterval - checkinJitter + c.randN(2*checkinJitter+1)
}

// tick makes one check-in attempt and returns the wait before the next.
func (c *checkinRunner) tick(ctx context.Context) time.Duration {
	if !c.ready() {
		return checkinReadyCheck
	}
	st := c.pinger.Status()
	if !st.State.ProEnabled() ||
		(c.blocked != "" && c.blocked == st.LID) {
		return c.interval()
	}
	c.blocked = ""
	err := c.ping(ctx, license.Ping{Checkins: true})
	var limited license.ErrRateLimited
	switch {
	case err == nil:
		c.remember(true)
	case license.StopsCheckins(err):
		c.blocked = st.LID
		klog.InfoS("cluster check-ins paused until the license changes",
			"reason", err.Error())
	case errors.As(err, &limited):
		return max(limited.RetryAfter, time.Minute)
	case ctx.Err() == nil:
		klog.V(2).InfoS("cluster check-in failed", "err", err.Error())
	}
	return c.interval()
}

func (c *checkinRunner) ping(ctx context.Context, p license.Ping) error {
	p.ClusterName, p.Namespace = c.clusterName, c.namespace
	pingCtx, cancel := context.WithTimeout(ctx, checkinTimeout)
	defer cancel()
	return c.pinger.Ping(pingCtx, p)
}

// announceOff sends checkins:false once after the operator turned
// check-ins off, so the app shows "off" rather than a false silence.
func (c *checkinRunner) announceOff(ctx context.Context) {
	var rec checkinRecord
	if found, err := c.disk.get(stateCheckins, &rec); err != nil ||
		!found || !rec.On {
		return
	}
	if !c.pinger.Status().State.ProEnabled() {
		return
	}
	if err := c.ping(ctx, license.Ping{Checkins: false}); err != nil {
		klog.InfoS("telling api.kwatch.dev check-ins are off failed",
			"err", err.Error())
		return
	}
	c.remember(false)
}

func (c *checkinRunner) remember(on bool) {
	var rec checkinRecord
	if found, err := c.disk.get(stateCheckins, &rec); err == nil &&
		found && rec.On == on {
		return
	}
	if err := c.disk.put(stateCheckins, checkinRecord{On: on}); err != nil {
		klog.ErrorS(err, "record cluster check-in state")
	}
}

// stopping sends the best-effort last check-in of a graceful shutdown.
// It never blocks shutdown longer than checkinStopTimeout; the next
// start's check-in clears it on the server.
func (c *checkinRunner) stopping() {
	if c == nil || !c.enabled || !c.pinger.Status().State.ProEnabled() {
		return
	}
	ctx, cancel := context.WithTimeout(
		context.Background(), checkinStopTimeout)
	defer cancel()
	p := license.Ping{Checkins: true, Stopping: true,
		ClusterName: c.clusterName, Namespace: c.namespace}
	if err := c.pinger.Ping(ctx, p); err != nil {
		klog.V(2).InfoS("shutdown check-in failed", "err", err.Error())
	}
}
