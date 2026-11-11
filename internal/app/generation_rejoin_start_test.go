package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/feature"
)

// Session starts that fail in a row back off before each rejoin, each
// rejoin is counted, and a session that started resets the backoff.
func TestFailedSessionStartsBackOffBeforeRejoining(t *testing.T) {
	before := rejoinCounts()
	h := newGenHarness(t)
	h.serve = func(gen int, deps *serverDeps) int {
		switch gen {
		case 1, 2, 3, 5:
			return failStartAndRejoin(deps)
		case 4:
			return loseLease(deps) // this session had started
		}
		return endGeneration(deps, nil)
	}

	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 6, h.gen)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second,
		4 * time.Second, time.Second}, h.sleeps.waits)
	require.Equal(t, before[0]+5, rejoinCounts()[0])
}

// A stop during the backoff ends the process without a new generation.
func TestStopDuringTheRejoinBackoffEnds(t *testing.T) {
	h := newGenHarness(t)
	h.stop = make(chan struct{})
	h.sleeps.onSleep = func(int) { close(h.stop) }
	h.sleeps.err = context.Canceled
	h.serve = func(_ int, deps *serverDeps) int {
		return failStartAndRejoin(deps)
	}

	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 1, h.gen, "no generation was built")
}

// An edition rebuild that also lost the Lease fences delivery: a manager
// that did not stop fails the process instead of rebuilding.
func TestEditionRebuildAfterALostLeaseFencesDelivery(t *testing.T) {
	h := newGenHarness(t)
	h.fired = true
	h.serve = func(gen int, deps *serverDeps) int {
		if gen > 1 {
			return endGeneration(deps, nil)
		}
		code := endGeneration(deps, restartTo(feature.ProEntitlements()))
		deps.leaseLost.Store(true)
		deps.deliveryManager = delivery.NewManagerWithDependencies(
			delivery.Dependencies{Clock: clock.RealClock{}})
		return code
	}

	require.Equal(t, 1, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 1, h.gen, "no generation was built")
}

// A rejoin retries Unauthorized and Forbidden on its startup call, which
// an auth webhook still down returns; a first start fails on them.
func TestRejoinRetriesAuthErrorsAFirstStartDoesNot(t *testing.T) {
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Resource: "kwatchconfigs"}, "",
		errors.New("webhook"))
	h := newGenHarness(t)
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return loseLease(deps)
		}
		return endGeneration(deps, nil)
	}
	h.apiErr = func(gen, attempt int) error {
		switch {
		case gen < 2 || attempt > 2:
			return nil
		case attempt == 1:
			return apierrors.NewUnauthorized("webhook")
		default:
			return forbidden
		}
	}
	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 2, h.gen)
	require.Len(t, h.apiSleeps.waits, 2)

	first := newGenHarness(t)
	first.apiErr = func(int, int) error { return forbidden }
	require.Equal(t, 1, first.env().runAll(feature.CommunityEntitlements()))
	require.Empty(t, first.apiSleeps.waits)
}

// A rejoin retries a NotFound for the KwatchConfig type for a while; a
// first start takes it at once.
func TestRejoinRetriesAMissingKwatchConfigType(t *testing.T) {
	missing := apierrors.NewNotFound(
		schema.GroupResource{Resource: "kwatchconfigs"}, "")
	h := newGenHarness(t)
	h.serve = func(gen int, deps *serverDeps) int {
		if gen == 1 {
			return loseLease(deps)
		}
		return endGeneration(deps, nil)
	}
	h.apiErr = func(gen, attempt int) error {
		if gen == 2 && attempt <= 2 {
			return missing
		}
		return nil
	}
	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Len(t, h.apiSleeps.waits, 2)

	first := newGenHarness(t)
	first.apiErr = func(int, int) error { return missing }
	_ = first.env().runAll(feature.CommunityEntitlements())
	require.Empty(t, first.apiSleeps.waits)
}

// A NotFound for the type is not a deleted KwatchConfig: the kept last
// good spec stays.
func TestMissingKwatchConfigTypeKeepsTheLastGoodSpec(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	_, outcome := startOverlay(t, goodSpec(), noLastGood)
	remember(t, path, outcome)

	t.Setenv("POD_NAMESPACE", "kwatch")
	_, outcome, err := applyStartupCRD(context.Background(), crdBase(),
		missingTypeClient(), nil, noLastGood, nil)
	require.NoError(t, err)
	require.True(t, outcome.typeMissing)
	remember(t, path, outcome)

	_, kept := keptAt(path)()
	require.True(t, kept, "a missing type does not clear the kept spec")
}

func missingTypeClient() *dynamicfake.FakeDynamicClient {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			kwatchConfigResource: "KwatchConfigList",
		})
	client.PrependReactor("list", kwatchConfigResource.Resource,
		func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewNotFound(schema.GroupResource{
				Resource: kwatchConfigResource.Resource}, "")
		})
	return client
}

// A stop while the generation after an edition rebuild is still starting
// releases the Lease the previous generation kept for it.
func TestStopWhileRebuildingReleasesTheKeptLease(t *testing.T) {
	var released atomic.Bool
	h := newGenHarness(t)
	h.stop = make(chan struct{})
	h.apiBlock = true
	h.serve = func(gen int, deps *serverDeps) int {
		code := endGeneration(deps, restartTo(feature.ProEntitlements()))
		deps.setLeaseRelease(func(context.Context) { released.Store(true) })
		return code
	}
	h.apiErr = func(gen, _ int) error {
		if gen == 1 {
			return nil
		}
		h.apiSleeps.onSleep = func(int) { close(h.stop) }
		return apierrors.NewServiceUnavailable("down")
	}

	require.Equal(t, 0, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 2, h.gen)
	require.True(t, released.Load(), "the kept Lease was released")
}

// A permission that stays refused ends the rejoin after the auth window
// instead of retrying forever.
func TestRejoinGivesUpOnAuthErrorsAfterItsWindow(t *testing.T) {
	h := newGenHarness(t)
	h.serve = func(_ int, deps *serverDeps) int { return loseLease(deps) }
	h.apiErr = func(gen, _ int) error {
		if gen == 1 {
			return nil
		}
		return apierrors.NewUnauthorized("token revoked")
	}

	require.Equal(t, 1, h.env().runAll(feature.CommunityEntitlements()))
	require.Equal(t, 2, h.gen)
	require.LessOrEqual(t, h.apiSleeps.total(), rebuildAuthWindow)
	require.NotEmpty(t, h.apiSleeps.waits)
}
