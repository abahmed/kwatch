package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/leaderelection"

	"github.com/abahmed/kwatch/internal/metrics"
)

// electWith runs one election on a fake elector with runner as the
// leader session. lose ends the term the way a failed renewal does.
func electWith(
	t *testing.T, deps *serverDeps, lose chan struct{},
	runner activeComponentRunner,
) error {
	t.Helper()
	t.Setenv("POD_NAME", "kwatch-a")
	t.Setenv("POD_NAMESPACE", "kwatch")
	t.Setenv("KWATCH_INSTALLATION_ID", "kwatch")
	factory := func(
		config leaderelection.LeaderElectionConfig,
	) (electionRunner, error) {
		return &fakeElection{callbacks: config.Callbacks, lose: lose}, nil
	}
	result := make(chan error, 1)
	go func() {
		result <- runLeaderElectionWithRunner(
			context.Background(), deps, factory, runner)
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the election did not end")
		return nil
	}
}

// A Lease lost while the session is still starting rejoins instead of
// failing on the startup step's "context canceled".
func TestLeaseLostWhileTheSessionStartsRejoins(t *testing.T) {
	deps := testServerDeps()
	lose := make(chan struct{})
	err := electWith(t, deps, lose,
		func(ctx context.Context, _ *serverDeps) error {
			close(lose)
			<-ctx.Done()
			return startFailure(fmt.Errorf(
				"state: read leader lease: %w", ctx.Err()))
		})

	requireRejoin(t, deps, err)
	require.True(t, deps.startFailed.Load(), "the next rejoin backs off")
}

// A required component that fails after the session started, while the
// Lease is held, still fails the process.
func TestSessionFailureWithTheLeaseHeldStillFails(t *testing.T) {
	deps := testServerDeps()
	err := electWith(t, deps, make(chan struct{}),
		func(context.Context, *serverDeps) error {
			return apierrors.NewServiceUnavailable("down")
		})

	_, code := componentExit(err)
	require.Equal(t, 1, code, "only a session start is rejoined")
}

// An API outage on a startup step gives up the term and rejoins; this
// pod still holds the Lease, so it is set up for release at a stop.
func TestOutageDuringTheSessionStartRejoins(t *testing.T) {
	losses := metrics.DefaultRegistry().LeadershipLosses.Load()
	for _, cause := range []error{
		apierrors.NewServiceUnavailable("down"),
		apierrors.NewInternalError(errors.New("etcd")),
		errNotLeaseHolder,
	} {
		deps := testServerDeps()
		err := electWith(t, deps, make(chan struct{}),
			func(context.Context, *serverDeps) error {
				return startFailure(fmt.Errorf(
					"state: read leader lease: %w", cause))
			})

		require.ErrorIs(t, err, errLeaseLostRejoin, cause.Error())
		require.Equal(t, rebuildLeadershipLost, deps.rebuildReason())
		require.True(t, deps.startFailed.Load())
		require.NotNil(t, deps.leaseRelease(), "the held Lease is released")
	}
	require.Equal(t, losses,
		metrics.DefaultRegistry().LeadershipLosses.Load(),
		"a term given up is not counted as a lost Lease")
}

// A startup step that fails for good still fails the process: a rejoin
// would only repeat it.
func TestPermanentSessionStartFailureStillFails(t *testing.T) {
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Resource: "leases"}, "", errors.New("rbac"))
	for _, cause := range []error{errors.New("bad volume"), forbidden} {
		deps := testServerDeps()
		err := electWith(t, deps, make(chan struct{}),
			func(context.Context, *serverDeps) error {
				return startFailure(fmt.Errorf("state: %w", cause))
			})

		require.ErrorIs(t, err, cause)
		_, code := componentExit(err)
		require.Equal(t, 1, code)
		require.Equal(t, rebuildNone, deps.rebuildReason())
	}
}

// An elector that returned on its own lost the Lease, even when the
// session asked for an edition rebuild: delivery is then fenced.
func TestEditionRestartWithALostLeaseMarksTheLoss(t *testing.T) {
	deps := testServerDeps()
	callbacks := &leaderCallbacks{activeErrors: make(chan error, 1)}
	callbacks.activeErrors <- errEditionChanged

	err := finishElection(context.Background(), deps, callbacks, nil, "a")

	require.ErrorIs(t, err, errEditionChanged)
	require.True(t, deps.leaseLost.Load())

	deps = testServerDeps()
	callbacks = &leaderCallbacks{activeErrors: make(chan error, 1)}
	callbacks.activeErrors <- errEditionChanged
	callbacks.ended.Store(true)
	_ = finishElection(context.Background(), deps, callbacks, nil, "a")
	require.False(t, deps.leaseLost.Load(), "a term this pod ended")
}

func TestRejoinBackoffDoublesToItsCap(t *testing.T) {
	got := []time.Duration{}
	for failed := 0; failed <= 7; failed++ {
		got = append(got, rejoinBackoff(failed))
	}
	require.Equal(t, []time.Duration{0, time.Second, 2 * time.Second,
		4 * time.Second, 8 * time.Second, 16 * time.Second,
		30 * time.Second, 30 * time.Second}, got)
}

// failStartAndRejoin ends a generation whose session never started.
func failStartAndRejoin(deps *serverDeps) int {
	deps.startFailed.Store(true)
	return loseLease(deps)
}
