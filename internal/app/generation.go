package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/version"
)

// generationResult is how one generation of the application ended. next and
// again are set when the license changed the edition and kwatch rebuilds.
type generationResult struct {
	code  int
	next  feature.Entitlements
	again bool
	// stopHealth frees the health port the generation kept serving while
	// it ended. The next generation calls it just before it binds the
	// port, so liveness answers through the whole rebuild. Nil when the
	// generation did not ask for a rebuild.
	stopHealth func()
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
		feature.Entitlements,
	) (*bootstrap, error)
	serve func(context.Context, *serverDeps) int
	// logs is the klog format shared by every generation of the process.
	logs *logFormat
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
	entitlements, rebuilding := first, false
	var stopHealth func()
	for {
		res := g.run(entitlements, rebuilding, stopHealth)
		if !res.again {
			return res.code
		}
		entitlements, rebuilding = res.next, true
		stopHealth = res.stopHealth
	}
}

// run builds and serves one generation of the application. It reports the
// entitlements to rebuild with when the license changed the edition.
// stopHealth is the previous generation's health listener, which stayed
// open; it is closed just before this generation binds the port.
func (g generationEnv) run(
	entitlements feature.Entitlements, rebuilding bool, stopHealth func(),
) generationResult {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var retry *rebuildRetry
	if rebuilding {
		retry = g.retry
	}
	deps, err := g.prepare(ctx, cancel, retry, entitlements, stopHealth)
	if err != nil {
		if stopHealth != nil {
			stopHealth()
		}
		return generationResult{code: 1}
	}
	if rebuilding {
		metrics.DefaultRegistry().IncEditionRebuild("success")
	}
	return g.finish(deps, g.serve(ctx, deps))
}

// prepare loads the configuration and builds the infrastructure of one
// generation. A failure is logged and leaves nothing running. On a rebuild
// only a transient failure is retried; a permanent one, such as an invalid
// configuration, fails the same way a first start does.
func (g generationEnv) prepare(
	ctx context.Context, cancel context.CancelFunc, retry *rebuildRetry,
	entitlements feature.Entitlements, stopHealth func(),
) (*serverDeps, error) {
	cfg, err := g.load()
	if err != nil {
		return nil, fail(retry, "failed to load config", "config", err)
	}
	g.logs.apply(cfg.Runtime.Application().LogFormatter)

	klog.InfoS(fmt.Sprintf(welcomeMessage, version.Short()))

	var boot *bootstrap
	err = retry.do(ctx, "bootstrap", func() (err error) {
		boot, err = g.boot(
			ctx, cfg, g.now, g.processStart, entitlements)
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
	if stopHealth != nil {
		stopHealth()
	}
	err = retry.do(ctx, "health listener", deps.healthServer.Open)
	if err != nil {
		releaseUnserved(deps)
		return nil, fail(retry, "failed to open health server",
			"health listener", err)
	}
	return deps, nil
}

// fail logs why a generation could not start. A rebuild says so and counts
// it; a first start keeps its own message.
func fail(retry *rebuildRetry, msg, stage string, err error) error {
	if retry == nil {
		klog.ErrorS(err, msg)
		return err
	}
	return failRebuild(stage, err)
}

// finish ends a generation. When the license asked for a rebuild it waits
// for the generation's resources to be released before the next one.
func (g generationEnv) finish(
	deps *serverDeps, code int,
) generationResult {
	if !deps.editionRestart.Load() {
		return generationResult{code: code}
	}
	logRebuild(deps)
	closeIdleConnections(deps.clients)
	if err := awaitRelease(deps, g.after); err != nil {
		_ = failRebuild("release", err)
		stopHealthServer(deps)
		return generationResult{code: 1}
	}
	return generationResult{
		code: code, next: deps.edition.Load(), again: true,
		stopHealth: func() { stopHealthServer(deps) },
	}
}

var errStateStoreHeld = errors.New(
	"the previous generation still holds the state file")

// awaitRelease waits, within the existing shutdown budgets, for the
// previous generation to let go of what the next one needs. The state file
// must be closed: reopening it while a writer still holds it only times out
// on the file lock, so a rebuild that cannot free it fails now and the pod
// restarts cleanly. The health port is freed by Stop (stopHealthServer) and
// a delivery manager that is still draining is cut off by its own
// deadline, so those only warn.
func awaitRelease(deps *serverDeps, after afterFunc) error {
	if store := deps.openedStore.Load(); store != nil &&
		!waitDone(store.Done(), storeReleaseWait, after) {
		recordShutdownTimeout("state-store")
		return errStateStoreHeld
	}
	if deps.deliveryManager != nil &&
		!waitDone(deps.deliveryManager.Done(), deliveryStopTimeout, after) {
		recordShutdownTimeout("delivery")
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
