package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/rbac"
)

func activeDeps(t *testing.T) *serverDeps {
	t.Helper()
	deps := pipelineDeps(t, &config.Config{})
	deps.securityMonitor = rbac.NewMonitor(deps.clients.Kubernetes,
		rbac.Checks("kwatch", "kwatch-leader", false), clock.RealClock{},
		reportPermissions(deps.healthServer))
	deps.heartbeat = heartbeat.NewHeartbeatMonitorWithRuntime(
		deps.runtime, http.DefaultClient, deps.healthServer.Ready)
	deps.clients.HTTP = http.DefaultClient
	return deps
}

func useLease(t *testing.T, deps *serverDeps) {
	t.Helper()
	useStateEnv(t)
	_, err := deps.clients.Kubernetes.CoordinationV1().Leases("kwatch").
		Create(context.Background(),
			leaseWithTransitions(ptr.To(int32(0))),
			metav1.CreateOptions{})
	require.NoError(t, err)
}

func TestRunActiveComponentsServesUntilLeadershipEnds(t *testing.T) {
	deps := activeDeps(t)
	useLease(t, deps)
	deps.readiness.begin(1, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)

	go func() { done <- runActiveComponents(ctx, deps) }()

	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond, "leader session never became ready")
	// Heartbeat is not configured, so it stops at once and is not shown
	// as running.
	require.Eventually(t, func() bool {
		_, shown := deps.healthServer.ComponentStatuses()["heartbeat"]
		return !shown
	}, 15*time.Second, 5*time.Millisecond, "disabled heartbeat shown")
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("active session did not stop when leadership ended")
	}
}

func TestRunActiveComponentsFailsWithoutLeaseAndReportsState(t *testing.T) {
	deps := activeDeps(t)
	t.Setenv("KWATCH_DATA_DIR", t.TempDir())
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-test-leader")

	err := runActiveComponents(context.Background(), deps)

	require.ErrorContains(t, err, "state:")
	status := deps.healthServer.ComponentStatuses()["state"]
	require.False(t, status.Available)
}
