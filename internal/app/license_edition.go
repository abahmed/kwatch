package app

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/license"
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
	// processStart replaces licenseProcessStart; tests only.
	processStart time.Time
	// now replaces the application clock for licensing; tests only.
	now func() time.Time
}

// licenseProcessStart is when this process started. A deactivation marker
// older than it was made for a previous run of the pod.
var licenseProcessStart = time.Now()

func (s *licenseSetup) processStartTime() time.Time {
	if !s.processStart.IsZero() {
		return s.processStart
	}
	return licenseProcessStart
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
func newLicenseSetup(runtime config.RuntimeConfig) *licenseSetup {
	cfg := runtime.Lifecycle().License()
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = license.DefaultEndpoint
	}
	return &licenseSetup{key: cfg.Key, endpoint: endpoint}
}
