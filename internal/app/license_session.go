package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/status"
	"github.com/abahmed/kwatch/internal/storage"
	"github.com/abahmed/kwatch/internal/version"
)

// licenseStopWait bounds how long ending a session waits for the license
// goroutines, so a stuck network call cannot hold the state file open.
const licenseStopWait = 5 * time.Second

// licenseSession is the license manager of one leader session. The leader
// owns it: standbys never contact the license server.
type licenseSession struct {
	deps    *serverDeps
	disk    diskState
	mgr     *license.Manager
	tracker *renewTracker
	updates <-chan license.Status
	now     func() time.Time
	// send delivers a notice to the alert providers.
	send func(string)
	// edition is the edition this session's components are built for.
	edition feature.Edition

	cur atomic.Pointer[license.Status]

	restartOnce sync.Once
	restart     chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
}

// startLicense creates the manager and, when a key is set, runs the first
// activation or renewal before returning (bounded by the setup's initial
// sync timeout), so a valid license is Pro when components are built. It
// returns nil when licensing is not wired in. With no key it makes no
// network call and writes no Secret.
func startLicense(
	ctx context.Context, deps *serverDeps, disk diskState,
) *licenseSession {
	setup := deps.license
	if setup == nil {
		return nil
	}
	now := deps.clients.Clock.Now
	if setup.now != nil {
		now = setup.now
	}
	l := &licenseSession{
		deps: deps, disk: disk, now: now,
		restart: make(chan struct{}),
		edition: feature.OrCommunity(deps.entitlements).Edition(),
	}
	l.tracker = &renewTracker{now: l.now}
	if deps.deliveryManager != nil {
		l.send = deps.deliveryManager.Notify
	}
	cfg := l.managerConfig(setup)
	l.mgr = license.NewManager(cfg)
	l.updates = l.mgr.Subscribe()
	l.mgr.Start(ctx)
	st := l.mgr.Status()
	l.record(st)
	deps.setEdition(entitlementsFor(st))
	if deps.healthServer != nil {
		deps.healthServer.SetLicenseSource(l.view)
	}
	return l
}

// editionMismatch reports whether the license now entitles a different
// edition than the one this session's components would be built for.
func (l *licenseSession) editionMismatch() bool {
	if l == nil {
		return false
	}
	return entitlementsFor(*l.cur.Load()).Edition() != l.edition
}

// watch runs until ctx ends: it follows the manager, keeps metrics and
// /status current, sends the notices and, when the edition changes and
// stays changed past the debounce, ends the session once.
func (l *licenseSession) watch(ctx context.Context) {
	if l == nil {
		return
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.loop(ctx)
	}()
}

func (l *licenseSession) loop(ctx context.Context) {
	tick := time.NewTicker(l.deps.license.tickEvery())
	defer tick.Stop()
	var debounce *time.Timer
	var fire <-chan time.Time
	stopDebounce := func() {
		if debounce != nil {
			debounce.Stop()
			debounce, fire = nil, nil
		}
	}
	defer stopDebounce()
	observe := func(st license.Status) {
		l.record(st)
		l.notify(st)
		ent := entitlementsFor(st)
		l.deps.setEdition(ent)
		switch {
		case ent.Edition() == l.edition:
			stopDebounce()
		case debounce == nil:
			debounce = time.NewTimer(l.deps.license.editionDebounce())
			fire = debounce.C
		}
	}
	observe(l.mgr.Status())
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-l.updates:
			observe(st)
		case <-tick.C:
			l.mgr.CheckDeactivated(ctx)
			l.mgr.CheckMove(ctx)
			observe(l.mgr.Status())
		case <-fire:
			debounce, fire = nil, nil
			st := l.mgr.Status()
			l.record(st)
			ent := entitlementsFor(st)
			l.deps.setEdition(ent)
			if ent.Edition() != l.edition {
				l.requestRestart(st)
				return
			}
		}
	}
}

// requestRestart ends the session, once, so components rebuild with the
// new entitlements.
func (l *licenseSession) requestRestart(st license.Status) {
	l.restartOnce.Do(func() {
		klog.InfoS("license state changed the edition; restarting the "+
			"monitoring session", "component", "license",
			"state", string(st.State), "lid", st.LID)
		l.deps.editionRestart.Store(true)
		close(l.restart)
	})
}

// restarted is closed when the license asked for a new session. It is nil
// (never ready) without licensing.
func (l *licenseSession) restarted() <-chan struct{} {
	if l == nil {
		return nil
	}
	return l.restart
}

// record publishes st to metrics and to /status.
func (l *licenseSession) record(st license.Status) {
	l.cur.Store(&st)
	reg := metrics.DefaultRegistry()
	reg.SetLicenseState(string(st.State))
	var expiry int64
	if !st.ExpiresAt.IsZero() {
		expiry = st.ExpiresAt.Unix()
	}
	reg.LicenseExpiry.Store(expiry)
}

// notify sends the notices st calls for. A notice is recorded before it is
// sent, so a failed write never repeats it; the record survives restarts.
func (l *licenseSession) notify(st license.Status) {
	if l.send == nil {
		return
	}
	var rec licenseNotified
	if _, err := l.disk.get(stateLicenseNotified, &rec); err != nil {
		klog.ErrorS(err, "read license notices", "component", "license")
		return
	}
	msgs, updated := licenseNotices(rec, st, l.now())
	if len(msgs) == 0 && updated == rec {
		return
	}
	if err := l.disk.put(stateLicenseNotified, updated); err != nil {
		klog.ErrorS(err, "record license notices", "component", "license")
		return
	}
	for _, msg := range msgs {
		l.send(msg)
	}
}

// view is the license section of /status.
func (l *licenseSession) view() *status.License {
	st := l.cur.Load()
	if st == nil {
		return nil
	}
	now := l.now()
	out := &status.License{
		Edition: editionLabel(l.deps.edition.Load().Edition()),
		State:   string(st.State), Plan: planLabel(st.Plan), LID: st.LID,
		Reason: st.Reason,
	}
	if !st.ExpiresAt.IsZero() {
		exp := st.ExpiresAt
		out.ExpiresAt = &exp
		days := max(0, int(exp.Sub(now)/(24*time.Hour)))
		out.DaysLeft = &days
	}
	if !st.GraceUntil.IsZero() {
		g := st.GraceUntil
		out.GraceUntil = &g
	}
	last, next := l.tracker.snapshot()
	if !last.IsZero() {
		out.LastRenew = &last
	}
	if !next.IsZero() && st.State != license.StateCommunity {
		out.NextAttempt = &next
	}
	if st.State == license.StateConflict {
		out.Hint = status.ConflictHint
	}
	return out
}

func editionLabel(e feature.Edition) string {
	if e == feature.ProEdition {
		return "Pro"
	}
	return "Community"
}

// stop waits for the license goroutines, bounded. The caller has already
// cancelled the session context.
func (l *licenseSession) stop() {
	if l == nil {
		return
	}
	l.stopOnce.Do(func() {
		done := make(chan struct{})
		go func() {
			l.wg.Wait()
			l.mgr.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(licenseStopWait):
			recordShutdownTimeout("license")
		}
		if l.deps.healthServer != nil {
			l.deps.healthServer.SetLicenseSource(nil)
		}
	})
}

func sleepLicense(ctx context.Context, d time.Duration) error {
	if !waitTelemetry(ctx, d) {
		return ctx.Err()
	}
	return nil
}

// closeEarly ends a session that built no components: it stops the license
// goroutines, then closes the state file they write to.
func closeEarly(
	cancel context.CancelFunc, lic *licenseSession, state *storage.Store,
) {
	cancel()
	lic.stop()
	closeStore(state)
}

// settleLicense settles the license before any component exists, so a
// valid license is Pro from the first build. When it entitles another
// edition than this generation was built for, the session ends at once
// with errEditionChanged and the application rebuilds with the new one.
func settleLicense(
	ctx context.Context, deps *serverDeps, cancel context.CancelFunc,
	disk diskState,
) (*licenseSession, error) {
	lic := startLicense(ctx, deps, disk)
	if lic.editionMismatch() {
		return nil, abandonForEdition(deps, cancel, lic, disk.store)
	}
	return lic, nil
}

// abandonForEdition ends the session before any component exists, because
// the license entitles a different edition than this generation was built
// for. The application rebuilds in process (see RunWithClock).
func abandonForEdition(
	deps *serverDeps, cancel context.CancelFunc, lic *licenseSession,
	state *storage.Store,
) error {
	deps.editionRestart.Store(true)
	deps.readiness.withdraw()
	closeEarly(cancel, lic, state)
	return errEditionChanged
}

// managerConfig builds the manager's configuration. The network side is
// set up only with a key: without one nothing reads Kubernetes for the
// license and nothing is written.
func (l *licenseSession) managerConfig(setup *licenseSetup) license.Config {
	deps := l.deps
	cfg := license.Config{
		Key:      setup.key,
		Verifier: setup.verifier,
		Version:  version.Short(),
		Now:      l.now,
		Sleep: func(ctx context.Context, d time.Duration) error {
			l.tracker.scheduled(d)
			return sleepLicense(ctx, d)
		},
		LeaseStore:         boltLeaseStore{disk: l.disk},
		InitialSyncTimeout: setup.syncTimeout(),
	}
	if setup.key == "" || deps.clients.Kubernetes == nil {
		return cfg
	}
	kube := deps.clients.Kubernetes
	cfg.Client = &license.Client{
		Endpoint:  setup.endpoint,
		HTTP:      licenseHTTPClient(deps.clients.HTTP, l.tracker),
		UserAgent: "kwatch-license/1",
	}
	cfg.InstanceStore = kubeInstanceStore{
		client: kube, namespace: kubeclient.GetNamespace(),
	}
	cfg.KubeSystemUID = kubeSystemUID(kube)
	cfg.NodeCount = nodeCount(kube)
	// The cluster id is derived from the live kube-system UID on every
	// use. It never falls back to a random or stored id: a wrong id would
	// bind the license to a phantom cluster and read as a conflict later.
	cfg.ClusterUUIDFrom = clusterIDFromUID
	instances := kubeInstanceStore{
		client: kube, namespace: kubeclient.GetNamespace(),
	}
	cfg.DeactivatedAt = instances.DeactivatedAt
	cfg.MoveRequestedAt = instances.MoveRequestedAt
	cfg.MoveFinished = instances.FinishMove
	cfg.ProcessStart = setup.processStartTime()
	return cfg
}
