package app

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/safego"
)

// errEditionChanged ends a leader session whose license state flipped the
// edition. The application then rebuilds in process (see RunWithClock) so
// every component is built with the new entitlements.
var errEditionChanged = errors.New("edition changed")

// editionHolder is the edition the installation currently runs. The
// leader updates it; the health server and standby replicas read it. The
// zero value is the Community edition.
type editionHolder struct {
	value atomic.Pointer[feature.Entitlements]
}

// Load returns the current entitlements, Community when none were set.
func (h *editionHolder) Load() feature.Entitlements {
	if p := h.value.Load(); p != nil {
		return feature.OrCommunity(*p)
	}
	return feature.CommunityEntitlements()
}

// Store replaces the current entitlements.
func (h *editionHolder) Store(e feature.Entitlements) {
	e = feature.OrCommunity(e)
	h.value.Store(&e)
}

// entitlementsFor is what a licensing status entitles. A test build may
// still override it (see edition_override_licensetest.go).
func entitlementsFor(st license.Status) feature.Entitlements {
	e := feature.CommunityEntitlements()
	if st.State.ProEnabled() {
		e = feature.ProEntitlements()
	}
	return editionOverride(e)
}

// Default licensing timings. Tests shorten them through licenseSetup.
const (
	// licenseInitialSync bounds the first activation before components
	// are built.
	licenseInitialSync = 10 * time.Second
	// licenseEditionDebounce is how long a changed edition must hold
	// before the session restarts for it.
	licenseEditionDebounce = 30 * time.Second
	// licenseTick re-evaluates the license against the clock, which is
	// what moves it into grace or expiry between renewals.
	licenseTick = time.Minute
)

// licenseSetup is the licensing wiring of one application run. A nil
// setup (a hand-built serverDeps in a test) runs without licensing.
type licenseSetup struct {
	key      string
	endpoint string
	// verifier is nil in production, which trusts the compiled-in keys.
	// Tests inject their own; no other path can add a trusted key.
	verifier    *license.Verifier
	initialSync time.Duration
	debounce    time.Duration
	tick        time.Duration
	// processStart is when this process started, taken from the
	// application clock once and kept across in-process rebuilds. A
	// deactivation marker older than it was made for a previous run of
	// the pod.
	processStart time.Time
	// now replaces the application clock for licensing; tests only.
	now func() time.Time
}

func (s *licenseSetup) processStartTime() time.Time {
	return s.processStart
}

func (s *licenseSetup) syncTimeout() time.Duration {
	if s.initialSync > 0 {
		return s.initialSync
	}
	return licenseInitialSync
}

func (s *licenseSetup) editionDebounce() time.Duration {
	if s.debounce > 0 {
		return s.debounce
	}
	return licenseEditionDebounce
}

func (s *licenseSetup) tickEvery() time.Duration {
	if s.tick > 0 {
		return s.tick
	}
	return licenseTick
}

// newLicenseSetup reads the license settings from the runtime snapshot.
func newLicenseSetup(
	runtime config.RuntimeConfig, processStart time.Time,
) *licenseSetup {
	cfg := runtime.Lifecycle().License()
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = license.DefaultEndpoint
	}
	return &licenseSetup{
		key: cfg.Key, endpoint: endpoint, processStart: processStart,
	}
}

// keepLoop runs the session's loop until ctx ends or it asked for a
// restart. A panic in it (a notice, a status no code expected) must not
// end the process, because licensing never takes monitoring down: it is
// logged and counted, and the loop starts again from the manager's
// current state, which still rests on the cached lease. The loop is
// ready only once it observed the license: one that panics at its
// start backs off and, after a few failures, shows on /health.
func (l *licenseSession) keepLoop(ctx context.Context) {
	safego.Keep(ctx, "license", func(ready func()) error {
		l.loop(ctx, ready)
		return nil
	}, safego.Options{Wait: l.restartWait, Sick: l.loopSick})
}

// loopSick shows on /health a loop that keeps failing. Its recovery
// needs nothing here: the loop's first observation records the
// license's own status.
func (l *licenseSession) loopSick(sick bool) {
	if !sick || l.deps == nil || l.deps.healthServer == nil {
		return
	}
	l.deps.healthServer.SetComponentStatus(licenseComponent, "degraded",
		"license_loop_failing", false)
}
