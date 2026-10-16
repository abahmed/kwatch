package app

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/storage"
)

func countLines(logged, needle string) int {
	return strings.Count(logged, needle)
}

func isClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func TestRebuildClosesTheStateFileBeforeTheNextGenerationOpensIt(t *testing.T) {
	h := newGenHarness(t)
	var first *storage.Store
	var path string
	var closedAtReopen bool
	h.serve = func(gen int, deps *serverDeps) int {
		if gen > 1 {
			return endGeneration(deps, nil)
		}
		first, path = holdStateFile(t, deps)
		closeStore(first) // the leader session's last step
		return endGeneration(deps, restartTo(feature.ProEntitlements()))
	}
	h.bootErr = func(gen, _ int) error {
		if gen == 2 {
			closedAtReopen = isClosed(first.Done())
			// The reopen the new leader session makes would time out
			// on the file lock if the old handle were still open.
			again, err := storage.Open(path,
				storage.Options{Now: time.Now})
			require.NoError(t, err)
			require.NoError(t, again.Close())
		}
		return nil
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.True(t, closedAtReopen, "state file open at the next build")
	require.Equal(t, 2, h.gen)
}

func TestRebuildFailsWhenThePreviousGenerationKeepsTheStateFile(
	t *testing.T,
) {
	out := captureKlog(t)
	before := rebuildCounts()
	h := newGenHarness(t)
	h.fired = true // the release wait runs out at once
	h.serve = func(_ int, deps *serverDeps) int {
		holdStateFile(t, deps) // never closed: an abandoned writer
		return endGeneration(deps, restartTo(feature.ProEntitlements()))
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 1, code)
	require.Equal(t, 1, h.gen, "no second generation was built")
	after := rebuildCounts()
	require.Equal(t, before[2]+1, after[2])
	logged := out.String()
	require.Contains(t, logged, "edition rebuild failed")
	require.Contains(t, logged, "still holds the state file")
}

func TestRebuildWaitsForALateStateFileClose(t *testing.T) {
	deps := &serverDeps{}
	s, _ := holdStateFile(t, deps)
	waited := false
	after := func(time.Duration) <-chan time.Time {
		waited = true
		closeStore(s) // the session finishes while the rebuild waits
		return make(chan time.Time)
	}

	require.NoError(t, awaitRelease(deps, after))
	require.True(t, waited)
	require.True(t, isClosed(s.Done()))
}

func TestRebuildLogsOneLineAndCountsSuccess(t *testing.T) {
	out := captureKlog(t)
	before := rebuildCounts()
	h := newGenHarness(t)
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return endGeneration(deps, restartTo(feature.ProEntitlements()))
		}
		require.Equal(t, feature.ProEdition,
			deps.entitlements.Edition(), "rebuilt with the new edition")
		return endGeneration(deps, nil)
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	after := rebuildCounts()
	require.Equal(t, before[0]+1, after[0], "one rebuild succeeded")
	require.Equal(t, before[2], after[2])
	require.Equal(t, 1, countLines(out.String(),
		"edition changed to Pro, rebuilding components"))
	require.Empty(t, h.sleeps.waits)
}

func TestRebuildRetriesATransientBuildFailureThenServes(t *testing.T) {
	out := captureKlog(t)
	before := rebuildCounts()
	h := newGenHarness(t)
	h.bootErr = func(gen, attempt int) error {
		if gen == 2 && attempt <= 2 {
			return addrInUse()
		}
		return nil
	}
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return endGeneration(deps, restartTo(feature.ProEntitlements()))
		}
		return endGeneration(deps, nil)
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second},
		h.sleeps.waits)
	after := rebuildCounts()
	require.Equal(t, before[1]+2, after[1], "retries")
	require.Equal(t, before[0]+1, after[0], "success")
	require.Equal(t, before[2], after[2], "no failure")
	require.Equal(t, 2, countLines(out.String(), "step failed; retrying"))
}

func TestRebuildRetriesABusyHealthPortUntilItIsFree(t *testing.T) {
	port := freeTCPPort(t)
	held, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	require.NoError(t, err)
	defer held.Close()
	h := newGenHarness(t)
	h.health = func(gen int) config.HealthCheck {
		return config.HealthCheck{Enabled: gen == 2, Port: port}
	}
	// The previous generation lets go of the port during the first wait.
	h.sleeps.onSleep = func(int) { require.NoError(t, held.Close()) }
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return endGeneration(deps, restartTo(feature.ProEntitlements()))
		}
		return endGeneration(deps, nil)
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, []time.Duration{time.Second}, h.sleeps.waits)
}

func TestRebuildPermanentConfigErrorExitsLikeAFirstStart(t *testing.T) {
	out := captureKlog(t)
	before := rebuildCounts()
	bad := errors.New("invalid configuration")
	h := newGenHarness(t)
	h.loadErr = func(gen int) error {
		if gen == 2 {
			return bad
		}
		return nil
	}
	h.serve = func(_ int, deps *serverDeps) int {
		return endGeneration(deps, restartTo(feature.ProEntitlements()))
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 1, code)
	require.Empty(t, h.sleeps.waits, "a permanent error is not retried")
	after := rebuildCounts()
	require.Equal(t, before[2]+1, after[2])
	require.Contains(t, out.String(), "edition rebuild failed")
	require.Contains(t, out.String(), "invalid configuration")
}

func TestFirstStartConfigErrorKeepsItsOwnMessage(t *testing.T) {
	out := captureKlog(t)
	before := rebuildCounts()
	h := newGenHarness(t)
	h.loadErr = func(int) error { return errors.New("invalid configuration") }

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 1, code)
	require.Equal(t, before, rebuildCounts(), "not a rebuild")
	require.Contains(t, out.String(), "failed to load config")
	require.NotContains(t, out.String(), "edition rebuild failed")
}

func TestRebuildGivesUpAfterTheRetryBudgetAndExits(t *testing.T) {
	out := captureKlog(t)
	before := rebuildCounts()
	h := newGenHarness(t)
	h.bootErr = func(gen, _ int) error {
		if gen == 2 {
			return lockHeld()
		}
		return nil
	}
	h.serve = func(_ int, deps *serverDeps) int {
		return endGeneration(deps, restartTo(feature.ProEntitlements()))
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 1, code)
	require.Equal(t, rebuildRetryBudget, h.sleeps.total())
	after := rebuildCounts()
	require.Equal(t, before[1]+5, after[1])
	require.Equal(t, before[2]+1, after[2])
	require.Contains(t, out.String(), "edition rebuild failed")
}

// A license that flips Pro, Community, Pro in quick succession must not
// run two rebuilds at once: each generation is built only after the
// previous one has served to its end and been released.
func TestRebuildDoubleFlipNeverOverlapsGenerations(t *testing.T) {
	h := newGenHarness(t)
	editions := []feature.Entitlements{
		feature.ProEntitlements(), feature.CommunityEntitlements(),
		feature.ProEntitlements(),
	}
	h.serve = func(gen int, deps *serverDeps) int {
		if gen <= len(editions) {
			return endGeneration(deps, restartTo(editions[gen-1]))
		}
		return endGeneration(deps, nil)
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 4, h.gen)
	want := []string{}
	for range 4 {
		want = append(want, "load", "boot", "serve", "served")
	}
	require.Equal(t, want, h.events, "strictly one generation at a time")
}

func TestRebuildRequestFromAFlippingLicenseIsOnlyHonouredOnce(t *testing.T) {
	deps := &serverDeps{}
	l := &licenseSession{deps: deps, restart: make(chan struct{})}
	for range 3 { // Pro, Community, Pro seen before the session ends
		l.requestRestart(license.Status{State: license.StateExpired})
		l.requestRestart(license.Status{State: license.StateActive})
	}
	require.True(t, isClosed(l.restarted()))
	require.True(t, deps.editionRestart.Load())
}
