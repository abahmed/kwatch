package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/config/crd"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/version"
)

// generationResult is how one generation of the application ended. next,
// again and reason are set when kwatch rebuilds: the license changed the
// edition, or the Lease was lost and the next generation rejoins.
type generationResult struct {
	code   int
	next   feature.Entitlements
	again  bool
	reason rebuildReason
	// stopHealth frees the health port the generation kept serving while
	// it ended. The next generation calls it just before it binds the
	// port, so liveness answers through the whole rebuild. Nil when the
	// generation did not ask for a rebuild.
	stopHealth func()
	// abandon releases what a generation that asked for a rebuild still
	// holds (the health port and the Lease) when no next one is built.
	abandon func()
	// report names, on the health listener still open, why the next
	// generation is not serving yet.
	report func(reason string)
	// startFailed is set when the session ended before it started its
	// components, so the next rejoin backs off.
	startFailed bool
}

// previousGeneration is what a rebuilt generation takes over from the one
// before it: the health listener that is still answering, and what it
// still holds when this one never gets to serve.
type previousGeneration struct {
	reason     rebuildReason
	stopHealth func()
	abandon    func()
	report     func(reason string)
	// failedStarts counts the session starts in a row that failed;
	// backoff is the wait before this rejoin (rejoinBackoff).
	failedStarts int
	backoff      time.Duration
}

// generationEnv holds the collaborators of the generation loop, so the
// rebuild sequence runs under test without a cluster.
type generationEnv struct {
	now          func() time.Time
	processStart time.Time
	// retry paces the retries of a rebuild; a first start never retries.
	retry *rebuildRetry
	after func(time.Duration) <-chan time.Time
	load  func() (*config.Config, error)
	boot  func(
		context.Context, *config.Config, func() time.Time, time.Time,
		feature.Entitlements, *crd.StartupRetry,
	) (*bootstrap, error)
	// apiSleep paces the startup API retries; nil uses a timer.
	apiSleep sleepFunc
	serve    func(context.Context, *serverDeps) int
	// logs is the klog format shared by every generation of the process.
	logs *logFormat
	// stop closes when the process is asked to stop (notifyStop); nil
	// makes runAll register the signal handler itself.
	stop <-chan struct{}
}

func newGenerationEnv(now func() time.Time) generationEnv {
	return generationEnv{
		// The process start is read once, so an in-process rebuild does
		// not move it.
		now: now, processStart: now(),
		retry: &rebuildRetry{sleep: sleepContext}, after: time.After,
		load: loadConfig, boot: newBootstrap, serve: serve,
		logs: newLogFormat(),
	}
}

// runAll builds and serves generations one at a time: the next starts only
// after the previous one returned and released what it held, so two
// rebuilds are never in flight.
func (g generationEnv) runAll(first feature.Entitlements) int {
	if g.stop == nil {
		stop, release := notifyStop()
		defer release()
		g.stop = stop
	}
	entitlements := first
	var prev previousGeneration
	for {
		res := g.run(entitlements, prev)
		if !res.again {
			return res.code
		}
		// A stop signal that arrived while the generation ended: there
		// is nothing to rebuild for.
		if stopRequested(g.stop) {
			klog.InfoS("shutting down instead of rebuilding")
			res.abandon()
			return res.code
		}
		entitlements = res.next
		prev = nextPrevious(res, prev.failedStarts)
	}
}

// run builds and serves one generation of the application. It reports the
// entitlements to rebuild with when a rebuild was asked for. prev holds
// the previous generation's health listener, which stayed open; it is
// closed just before this generation binds the port.
func (g generationEnv) run(
	entitlements feature.Entitlements, prev previousGeneration,
) generationResult {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cancelOnStop(ctx, g.stop, cancel)
	var retry *rebuildRetry
	if prev.reason != rebuildNone {
		retry = g.retry.forReason(prev.reason)
	}
	err := errStoppedBeforeRejoin
	var deps *serverDeps
	if g.waitBeforeRejoin(ctx, prev) {
		deps, err = g.prepare(ctx, cancel, retry, entitlements, prev)
	}
	if err != nil {
		// The previous generation still holds the port, and after an
		// edition rebuild the Lease: nothing takes them over now.
		prev.release()
		if stopRequested(g.stop) {
			klog.InfoS("stopped while starting")
			return generationResult{}
		}
		return generationResult{code: 1}
	}
	deps.stop = g.stop
	if retry != nil {
		retry.succeed()
	}
	return g.finish(deps, g.serve(ctx, deps))
}

// prepare loads the configuration and builds the infrastructure of one
// generation. A failure is logged and leaves nothing running. On a rebuild
// only a transient failure is retried; a permanent one, such as an invalid
// configuration, fails the same way a first start does.
func (g generationEnv) prepare(
	ctx context.Context, cancel context.CancelFunc, retry *rebuildRetry,
	entitlements feature.Entitlements, prev previousGeneration,
) (*serverDeps, error) {
	cfg, err := g.load()
	if err != nil {
		return nil, fail(retry, "failed to load config", "config", err)
	}
	g.logs.apply(cfg.Runtime.Application().LogFormatter)

	klog.InfoS(fmt.Sprintf(welcomeMessage, version.Short()))

	var boot *bootstrap
	err = retry.do(ctx, "bootstrap", func() (err error) {
		boot, err = g.boot(ctx, cfg, g.now, g.processStart,
			entitlements, g.apiRetry(retry != nil, prev.report))
		return err
	})
	if err != nil {
		return nil, fail(retry, "failed to initialize application",
			"bootstrap", err)
	}
	// The KwatchConfig overlay may change app.logFormatter.
	g.logs.apply(boot.runtime.Application().LogFormatter)
	logSecretsAccess(boot.runtime, entitlements)
	logNoProviders(boot.runtime)
	deps := newServerDeps(ctx, cancel, boot)
	if !deps.licenseKeyConfigured() {
		deps.logEdition()
	}
	deps.rebuild = retry
	// Everything slow is built; only now does the previous generation let
	// go of the port, so the gap with no listener is as short as it can be.
	if prev.stopHealth != nil {
		prev.stopHealth()
	}
	err = retry.do(ctx, "health listener", deps.healthServer.Open)
	if err != nil {
		releaseUnserved(deps)
		return nil, fail(retry, "failed to open health server",
			"health listener", err)
	}
	openDashboardEarly(ctx, deps)
	return deps, nil
}

// fail logs why a generation could not start. A rebuild says so and counts
// it; a first start keeps its own message.
func fail(retry *rebuildRetry, msg, stage string, err error) error {
	if retry == nil {
		klog.ErrorS(err, msg)
		return err
	}
	return retry.fail(stage, err)
}

// cancelOnStop ends the generation's context at a stop signal, so a build
// in progress and its retries give up at once.
func cancelOnStop(
	ctx context.Context, stop <-chan struct{}, cancel context.CancelFunc,
) {
	select {
	case <-stop:
		cancel()
	case <-ctx.Done():
	}
}

// finish ends a generation. When the license asked for a rebuild, or the
// Lease was lost, it waits for the generation's resources to be released
// before the next one; a release it cannot prove fails the process. A
// generation that a stop signal or its canceled context ended is never
// rebuilt, even when the license asked for a rebuild just before: the
// signal is already consumed, so a new generation would run until it is
// killed.
func (g generationEnv) finish(
	deps *serverDeps, code int,
) generationResult {
	reason := deps.rebuildReason()
	if deps.stopped.Load() || reason == rebuildNone {
		return generationResult{code: code}
	}
	logRebuild(deps, reason)
	closeIdleConnections(deps.clients)
	// A lost Lease fences delivery whatever the reason reported: an
	// edition change can win over a loss in rebuildReason.
	err := awaitRelease(deps, g.after,
		deps.leaseLost.Load() || reason == rebuildLeadershipLost)
	if err != nil {
		_ = g.retry.forReason(reason).fail("release", err)
		stopHealthServer(deps)
		return generationResult{code: 1}
	}
	return generationResult{
		code: code, next: deps.edition.Load(), again: true,
		reason:     reason,
		stopHealth: func() { stopHealthServer(deps) },
		abandon: func() {
			stopHealthServer(deps)
			releaseLeaseAfterShutdown(deps, deps.ctx)
		},
		report: func(why string) {
			deps.healthServer.SetComponentStatus(
				"leader-election", "degraded", why, false)
		},
		startFailed: reason == rebuildLeadershipLost &&
			deps.startFailed.Load(),
	}
}

var (
	errStateStoreHeld = errors.New(
		"the previous generation still holds the state file")
	errDeliveryRunning = errors.New(
		"the previous generation's delivery did not stop")
)

// awaitRelease waits, within the existing shutdown budgets, for the
// previous generation to let go of what the next one needs. The state file
// must be closed: reopening it while a writer still holds it only times out
// on the file lock, so a rebuild that cannot free it fails now and the pod
// restarts cleanly. The health port is freed by Stop (stopHealthServer).
// A delivery manager that is still draining is cut off by its own
// deadline, so for an edition rebuild, which keeps the Lease, it only
// warns. After a lost Lease (fenceDelivery) it fails: the store fences
// late writes, but nothing fences an HTTP send still in flight while
// another replica may already lead, so only an exit proves it stopped.
func awaitRelease(
	deps *serverDeps, after afterFunc, fenceDelivery bool,
) error {
	if store := deps.openedStore.Load(); store != nil &&
		!waitDone(store.Done(), storeReleaseWait, after) {
		recordShutdownTimeout("state-store")
		return errStateStoreHeld
	}
	if deps.deliveryManager != nil &&
		!waitDone(deps.deliveryManager.Done(), deliveryStopTimeout, after) {
		recordShutdownTimeout("delivery")
		if fenceDelivery {
			return errDeliveryRunning
		}
	}
	return nil
}

type afterFunc = func(time.Duration) <-chan time.Time

// waitDone reports whether done closed within budget.
func waitDone(
	done <-chan struct{}, budget time.Duration, after afterFunc,
) bool {
	select {
	case <-done:
		return true
	default:
	}
	select {
	case <-done:
		return true
	case <-after(budget):
		return false
	}
}

// releaseUnserved releases a generation that was built but never served:
// the delivery manager and the health listener.
func releaseUnserved(deps *serverDeps) {
	deps.cancel()
	stopHealthServer(deps)
	ctx, cancel := boundedShutdownContext(deps.ctx, deliveryStopTimeout)
	defer cancel()
	if err := deps.deliveryManager.Stop(ctx); err != nil {
		recordShutdownTimeout("delivery")
	}
	closeIdleConnections(deps.clients)
}

// closeIdleConnections drops the keep-alive connections of a generation's
// HTTP clients so their goroutines do not outlive it.
func closeIdleConnections(clients kubeclient.ClientSet) {
	if clients.HTTP != nil {
		clients.HTTP.CloseIdleConnections()
	}
	if clients.PlatformHTTP != nil {
		clients.PlatformHTTP.CloseIdleConnections()
	}
	if clients.ProbeHTTP != nil {
		clients.ProbeHTTP.CloseIdleConnections()
	}
}
