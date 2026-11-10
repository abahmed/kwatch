package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/metrics"
)

// rejoinCounts is the success and failed rejoin counters.
func rejoinCounts() [2]int64 {
	r := metrics.DefaultRegistry()
	return [2]int64{
		r.LeadershipRejoins[0].Load(), r.LeadershipRejoins[1].Load(),
	}
}

// leaderElectionReason is the leader-election reason that /health on
// port reports, or "" when there is none or nothing answers.
func leaderElectionReason(port int) string {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var body struct {
		Components map[string]struct {
			Reason string `json:"reason"`
		} `json:"components"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return ""
	}
	return body.Components["leader-election"].Reason
}

func healthOn(port int) func(int) config.HealthCheck {
	return func(int) config.HealthCheck {
		return config.HealthCheck{Enabled: true, Port: port}
	}
}

// A lost Lease rebuilds in process with the entitlements it ran with,
// and the old listener keeps answering liveness, naming the loss, until
// the next generation takes the port.
func TestLeaseLossRejoinsWithTheSameEntitlements(t *testing.T) {
	port := freeTCPPort(t)
	before, edition := rejoinCounts(), rebuildCounts()
	h := newGenHarness(t)
	h.health = healthOn(port)
	var rebuilt feature.Edition
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return loseLease(deps)
		}
		rebuilt = deps.entitlements.Edition()
		return endGeneration(deps, nil)
	}
	var live int
	var reason string
	h.bootErr = func(gen, _ int) error {
		if gen == 2 {
			live = statusOf(port, "/healthz")
			reason = leaderElectionReason(port)
		}
		return nil
	}

	code := h.env().runAll(feature.ProEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 2, h.gen, "a lost Lease rejoins in a new generation")
	require.Equal(t, feature.ProEdition, rebuilt)
	require.Equal(t, http.StatusOK, live)
	require.Equal(t, "leadership_lost", reason)
	require.Equal(t, before[0]+1, rejoinCounts()[0], "rejoin counted")
	require.Equal(t, edition, rebuildCounts(), "not an edition rebuild")
}

// A Lease lost again in the rejoined generation rejoins again.
func TestLeaseLostAgainRejoinsAgain(t *testing.T) {
	h := newGenHarness(t)
	h.serve = func(gen int, deps *serverDeps) int {
		if gen < 3 {
			return loseLease(deps)
		}
		return endGeneration(deps, nil)
	}

	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 3, h.gen)
}

// While the API server is down the rejoin keeps retrying the startup
// call, the old listener names the outage, and the rejoin then serves.
func TestRejoinRetriesAnUnreachableAPIThenServes(t *testing.T) {
	port := freeTCPPort(t)
	h := newGenHarness(t)
	h.health = healthOn(port)
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return loseLease(deps)
		}
		return endGeneration(deps, nil)
	}
	var reason string
	h.apiErr = func(gen, attempt int) error {
		switch {
		case gen < 2 || attempt > 3:
			return nil
		case attempt == 3:
			reason = leaderElectionReason(port)
			return apierrors.NewServiceUnavailable("down")
		case attempt == 2:
			return fmt.Errorf("dial: %w", syscall.ECONNREFUSED)
		default:
			return apierrors.NewServiceUnavailable("down")
		}
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 2, h.gen)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second,
		4 * time.Second}, h.apiSleeps.waits)
	require.Equal(t, "api_unreachable", reason)
}

// The rejoin's retry has no budget: the old listener keeps answering, so
// it outlasts the first start's budget and still serves.
func TestRejoinRetriesBeyondTheFirstStartBudget(t *testing.T) {
	h := newGenHarness(t)
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return loseLease(deps)
		}
		return endGeneration(deps, nil)
	}
	h.apiErr = func(gen, attempt int) error {
		if gen == 2 && attempt <= 15 {
			return apierrors.NewServiceUnavailable("down")
		}
		return nil
	}

	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 2, h.gen)
	require.Greater(t, h.apiSleeps.total(), firstStartAPIBudget)
}

// A first start retries the startup call the same way.
func TestFirstStartRetriesAnUnreachableAPIThenServes(t *testing.T) {
	h := newGenHarness(t)
	h.apiErr = func(_, attempt int) error {
		if attempt <= 2 {
			return fmt.Errorf("dial: %w", syscall.ECONNREFUSED)
		}
		return nil
	}

	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 1, h.gen)
	require.Len(t, h.apiSleeps.waits, 2)
}

// A first start has no listener for the startupProbe, so it gives up
// within its budget and exits 1 as before.
func TestFirstStartGivesUpOnTheAPIAfterItsBudget(t *testing.T) {
	out := captureKlog(t)
	h := newGenHarness(t)
	h.apiErr = func(int, int) error {
		return apierrors.NewServiceUnavailable("down")
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 1, code)
	require.Equal(t, 1, h.gen)
	total := h.apiSleeps.total()
	require.LessOrEqual(t, total, firstStartAPIBudget)
	require.Greater(t, total+startupBackoffCap, firstStartAPIBudget)
	require.Contains(t, out.String(), "failed to initialize application")
}

// startupBackoffCap mirrors the crd package's longest retry wait.
const startupBackoffCap = 30 * time.Second

// A stop while the rejoin waits for the API ends the process at once:
// the rebuild retry has no budget of its own.
func TestStopDuringTheRejoinAPIRetryReturnsPromptly(t *testing.T) {
	h := newGenHarness(t)
	h.stop = make(chan struct{})
	h.apiBlock = true
	h.serve = func(_ int, deps *serverDeps) int {
		return loseLease(deps)
	}
	h.apiErr = func(gen, _ int) error {
		if gen == 1 {
			return nil
		}
		// The stop arrives while the retry waits for its next attempt.
		h.apiSleeps.onSleep = func(int) { close(h.stop) }
		return apierrors.NewServiceUnavailable("down")
	}
	done := make(chan int, 1)

	go func() { done <- h.env().runAll(feature.CommunityEntitlements()) }()

	select {
	case code := <-done:
		require.Equal(t, 0, code)
		require.Equal(t, 2, h.gen)
		require.Len(t, h.apiSleeps.waits, 1)
	case <-time.After(10 * time.Second):
		t.Fatal("a stop during the API retry did not end the run")
	}
}

// After a lost Lease a delivery manager that did not stop fails the
// process: an HTTP send in flight cannot be fenced by the state store.
func TestRejoinFailsWhenDeliveryDidNotStop(t *testing.T) {
	out := captureKlog(t)
	before := rejoinCounts()
	h := newGenHarness(t)
	h.fired = true // every release wait gives up at once
	h.serve = func(gen int, deps *serverDeps) int {
		if gen > 1 {
			return endGeneration(deps, nil)
		}
		code := loseLease(deps)
		// A manager whose workers never finished: Done stays open.
		deps.deliveryManager = delivery.NewManagerWithDependencies(
			delivery.Dependencies{Clock: clock.RealClock{}})
		return code
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 1, code)
	require.Equal(t, 1, h.gen, "no generation was built")
	require.Equal(t, before[1]+1, rejoinCounts()[1])
	require.Contains(t, out.String(), "leadership rejoin failed")
	require.Contains(t, out.String(), "delivery did not stop")
}

func TestAwaitReleaseFencesDeliveryOnlyAfterALostLease(t *testing.T) {
	deps := &serverDeps{deliveryManager: delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}})}
	fired := func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Time{}
		return ch
	}

	require.NoError(t, awaitRelease(deps, fired, false),
		"an edition rebuild keeps the Lease and only warns")
	require.ErrorIs(t, awaitRelease(deps, fired, true), errDeliveryRunning)
}

// A session that did not stop in time fails the process instead of
// rejoining: nothing proves it stopped sending and writing.
func TestSessionThatDidNotStopIsNotARejoin(t *testing.T) {
	deps := testServerDeps()
	callbacks := &leaderCallbacks{
		activeErrors: make(chan error, 1),
		activeDone:   make(chan struct{}), // never closed
		sessionWait:  10 * time.Millisecond,
	}
	callbacks.started.Store(true)

	err := finishElection(context.Background(), deps, callbacks, nil, "a")

	require.ErrorContains(t, err, "did not stop")
	_, code := componentExit(err)
	require.Equal(t, 1, code)
	require.Equal(t, rebuildNone, deps.rebuildReason())
}

// The state file still held after a lost Lease fails the rejoin.
func TestRejoinFailsWhenTheStateFileIsStillHeld(t *testing.T) {
	h := newGenHarness(t)
	h.fired = true
	h.serve = func(_ int, deps *serverDeps) int {
		holdStateFile(t, deps) // never closed: an abandoned writer
		return loseLease(deps)
	}

	require.Equal(t, 1, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 1, h.gen)
}
