package app

import (
	"os"
	"os/signal"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// Rebuilding must not leave the old generation's goroutines behind: the
// health server, the supervisor and the delivery manager all end with it.
// warmSignals starts the os/signal loop goroutine, which lives for the rest
// of the process once any caller has asked for a signal.
func warmSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	signal.Stop(ch)
}

func TestRebuildLeavesNoGoroutinesBehind(t *testing.T) {
	port := freeTCPPort(t)
	h := newGenHarness(t)
	h.health = func(int) config.HealthCheck {
		return config.HealthCheck{Enabled: true, Port: port}
	}
	h.serve = func(gen int, deps *serverDeps) int {
		require.NoError(t, deps.deliveryManager.Start(deps.ctx))
		if gen < 4 {
			return endGeneration(deps, restartTo(feature.ProEntitlements()))
		}
		return endGeneration(deps, nil)
	}
	warmSignals()
	baseline := runtime.NumGoroutine()

	code := h.env().runAll(feature.CommunityEntitlements())

	require.Equal(t, 0, code)
	require.Equal(t, 4, h.gen)
	require.Eventually(t, settledAt(baseline), 3*time.Second,
		10*time.Millisecond, "goroutines leaked")

}

// A generation that fails while it is being built releases what it built.
func TestRebuildFailureReleasesTheBuiltGeneration(t *testing.T) {
	port := freeTCPPort(t)
	h := newGenHarness(t)
	h.health = func(gen int) config.HealthCheck {
		return config.HealthCheck{Enabled: true, Port: port}
	}
	h.serve = func(gen int, deps *serverDeps) int {
		return endGeneration(deps, restartTo(feature.ProEntitlements()))
	}
	warmSignals()
	baseline := runtime.NumGoroutine()
	// Generation two can never bind: another listener owns the port.
	h.bootErr = func(gen, _ int) error { return nil }
	hold := freeHolder(t, port, h)

	code := h.env().runAll(feature.CommunityEntitlements())

	hold()
	require.Equal(t, 1, code)
	require.Equal(t, rebuildRetryBudget, h.sleeps.total())
	require.Eventually(t, settledAt(baseline), 3*time.Second,
		10*time.Millisecond, "goroutines leaked")
}

// settledAt reports whether the goroutine count is back to baseline. The
// Eventually helper runs the condition in a goroutine of its own, which
// the allowance of one covers; a leaking generation adds several.
func settledAt(baseline int) func() bool {
	return func() bool { return runtime.NumGoroutine() <= baseline+1 }
}
