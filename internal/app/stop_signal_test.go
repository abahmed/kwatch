package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
)

// askRebuild marks the generation the way the license does when it flips
// the edition: the leader session starts to end, nothing reached the
// supervisor's error channel yet.
func askRebuild(deps *serverDeps) {
	deps.editionRestart.Store(true)
	deps.edition.Store(feature.ProEntitlements())
}

// A stop that arrives while the leader session drains for an edition
// rebuild ends the process: the signal is consumed, so a rebuilt
// generation would run until it is killed.
func TestStopDuringEditionRebuildEndsTheProcess(t *testing.T) {
	h := newGenHarness(t)
	h.serve = func(gen int, deps *serverDeps) int {
		if gen > 1 {
			return endGeneration(deps, nil)
		}
		askRebuild(deps)
		deps.cancel() // the application is asked to stop
		return waitShutdown(deps, newComponentSupervisor(time.Now))
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 1, h.gen, "a stopped generation was rebuilt")
}

// The same stop still waits for the draining session and releases the
// Lease: the rebuild shortcut (no wait, Lease kept) is only for a rebuild
// the ended session itself reported.
func TestStopSignalDuringEditionRebuildWaitsForTheSession(t *testing.T) {
	h := newGenHarness(t)
	h.stop = make(chan struct{})
	var drained, released atomic.Bool
	h.serve = func(_ int, deps *serverDeps) int {
		supervisor := newComponentSupervisor(time.Now)
		supervisor.startOwned(deps.ctx, componentSpec{
			name: "leader-election",
			run: func(ctx context.Context) error {
				<-ctx.Done()
				time.Sleep(50 * time.Millisecond) // the session drains
				drained.Store(true)
				return nil
			},
		})
		deps.setLeaseRelease(func(context.Context) { released.Store(true) })
		askRebuild(deps)
		close(h.stop)
		code := waitShutdown(deps, supervisor)
		require.True(t, drained.Load(), "did not wait for the session")
		return code
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 1, h.gen, "a stopped generation was rebuilt")
	require.True(t, released.Load(), "the Lease was not released")
}

// A rebuild the session reported keeps the Lease for the next generation.
func TestEditionRebuildKeepsTheLease(t *testing.T) {
	h := newGenHarness(t)
	var released atomic.Bool
	h.serve = func(gen int, deps *serverDeps) int {
		if gen > 1 {
			return endGeneration(deps, nil)
		}
		deps.setLeaseRelease(func(context.Context) { released.Store(true) })
		return endGeneration(deps, restartTo(feature.ProEntitlements()))
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 2, h.gen)
	require.False(t, released.Load(), "a rebuild released the Lease")
}

// A stop that arrives after the generation ended for a rebuild, before
// the next one is built, builds nothing and releases the Lease.
func TestStopBetweenGenerationsBuildsNothing(t *testing.T) {
	h := newGenHarness(t)
	h.stop = make(chan struct{})
	var released atomic.Bool
	h.serve = func(_ int, deps *serverDeps) int {
		deps.setLeaseRelease(func(context.Context) { released.Store(true) })
		code := endGeneration(deps, restartTo(feature.ProEntitlements()))
		close(h.stop)
		return code
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 1, h.gen, "a generation was built after the stop")
	require.True(t, released.Load(), "the Lease was not released")
}

// A stop while a generation is being built ends the run without calling
// it a failure.
func TestStopWhileBuildingIsNotAFailure(t *testing.T) {
	h := newGenHarness(t)
	h.stop = make(chan struct{})
	h.bootErr = func(int, int) error {
		close(h.stop)
		return errors.New("bootstrap interrupted")
	}

	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 1, h.gen)
}

// The signal handler is registered for the whole run, so a real SIGTERM
// between two generations is neither lost nor left to kill the process:
// the next generation sees it and shuts down.
func TestSignalBetweenGenerationsIsNotLost(t *testing.T) {
	h := newGenHarness(t)
	h.loadErr = func(gen int) error {
		if gen == 2 {
			require.NoError(t,
				syscall.Kill(os.Getpid(), syscall.SIGTERM))
		}
		return nil
	}
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return endGeneration(deps,
				restartTo(feature.ProEntitlements()))
		}
		// Nothing ends this generation but the signal.
		return waitShutdown(deps, newComponentSupervisor(time.Now))
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 2, h.gen)
}

// A real SIGTERM while the next generation rejoins after a lost Lease
// ends the process cleanly.
func TestSignalDuringARejoinEndsTheProcess(t *testing.T) {
	h := newGenHarness(t)
	h.loadErr = func(gen int) error {
		if gen == 2 {
			require.NoError(t,
				syscall.Kill(os.Getpid(), syscall.SIGTERM))
		}
		return nil
	}
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return loseLease(deps)
		}
		return waitShutdown(deps, newComponentSupervisor(time.Now))
	}

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 2, h.gen)
}

// A real SIGTERM while the rejoin waits for the API server ends the
// unbounded retry at once.
func TestSignalDuringTheAPIRetryEndsTheProcess(t *testing.T) {
	h := newGenHarness(t)
	h.apiBlock = true
	h.serve = func(_ int, deps *serverDeps) int { return loseLease(deps) }
	h.apiErr = func(gen, _ int) error {
		if gen == 1 {
			return nil
		}
		h.apiSleeps.onSleep = func(int) {
			require.NoError(t,
				syscall.Kill(os.Getpid(), syscall.SIGTERM))
		}
		return fmt.Errorf("dial: %w", syscall.ECONNREFUSED)
	}
	done := make(chan int, 1)

	go func() { done <- h.env().runAll(feature.CommunityEntitlements()) }()

	select {
	case code := <-done:
		require.Equal(t, 0, code)
		require.Equal(t, 2, h.gen)
	case <-time.After(10 * time.Second):
		t.Fatal("SIGTERM during the API retry did not end the run")
	}
}

func TestNotifyStopClosesOnceAtSIGTERM(t *testing.T) {
	stop, release := notifyStop()
	defer release()
	require.False(t, stopRequested(stop))
	require.False(t, stopRequested(nil))

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	select {
	case <-stop:
	case <-time.After(5 * time.Second):
		t.Fatal("the stop signal was not delivered")
	}
}
