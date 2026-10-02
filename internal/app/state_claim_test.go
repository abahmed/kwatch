package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

const testPodName = "kwatch-0"

// leaseWithTransitions is a leader Lease held by testPodName.
func leaseWithTransitions(n *int32) *coordinationv1.Lease {
	return leaseHeldBy(testPodName, n)
}

func leaseHeldBy(holder string, n *int32) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kwatch-test-leader", Namespace: "kwatch",
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: ptr.To(holder), LeaseTransitions: n,
		},
	}
}

func epochDeps(objects ...*coordinationv1.Lease) *serverDeps {
	clientset := fake.NewSimpleClientset()
	for _, lease := range objects {
		_, _ = clientset.CoordinationV1().Leases("kwatch").Create(
			context.Background(), lease, metav1.CreateOptions{})
	}
	return &serverDeps{clients: kubeclient.ClientSet{
		Kubernetes: clientset, Clock: clock.RealClock{},
	}}
}

func useStateEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KWATCH_DATA_DIR", dir)
	t.Setenv("POD_NAMESPACE", "kwatch")
	t.Setenv("POD_NAME", testPodName)
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-test-leader")
	return dir
}

func TestOpenStoreClaimsNextStoreEpoch(t *testing.T) {
	dir := useStateEnv(t)
	deps := epochDeps(leaseWithTransitions(ptr.To(int32(4))))

	first, err := openStore(context.Background(), deps)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, "state.db"))
	require.Equal(t, uint64(1), first.Epoch())
	closeStore(first)
	second, err := openStore(context.Background(), deps)
	require.NoError(t, err)
	defer closeStore(second)

	require.Equal(t, uint64(2), second.Epoch())
}

// A Lease that was deleted or renamed restarts its transition count while
// the state volume keeps the old epoch. The new holder must still claim.
func TestOpenStoreSucceedsAfterLeaseReset(t *testing.T) {
	useStateEnv(t)
	old := epochDeps(leaseWithTransitions(ptr.To(int32(9))))
	for range 3 {
		s, err := openStore(context.Background(), old)
		require.NoError(t, err)
		closeStore(s)
	}
	fresh := epochDeps(leaseWithTransitions(nil))

	s, err := openStore(context.Background(), fresh)

	require.NoError(t, err, "a fresh Lease must not brick the store")
	defer closeStore(s)
	require.Equal(t, uint64(4), s.Epoch())
	require.NoError(t, diskState{store: s}.put("k", "v"))
}

func TestOpenStoreRejectsReplicaThatDoesNotHoldLease(t *testing.T) {
	useStateEnv(t)
	deps := epochDeps(leaseHeldBy("kwatch-1", nil))

	_, err := openStore(context.Background(), deps)

	require.ErrorIs(t, err, errNotLeaseHolder)
}

func TestOpenStoreFailsWithoutLease(t *testing.T) {
	useStateEnv(t)

	_, err := openStore(context.Background(), epochDeps())

	require.ErrorContains(t, err, "read leader lease")
}

func TestOpenStoreStopsWithCanceledContext(t *testing.T) {
	useStateEnv(t)
	deps := epochDeps(leaseWithTransitions(nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deps.clients.Kubernetes.(*fake.Clientset).PrependReactor("get",
		"leases", func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, ctx.Err()
		})

	_, err := openStore(ctx, deps)

	require.ErrorIs(t, err, context.Canceled)
}

func TestOpenStoreFailsWhenDataDirIsAFile(t *testing.T) {
	useStateEnv(t)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	t.Setenv("KWATCH_DATA_DIR", file)

	_, err := openStore(context.Background(), epochDeps())

	require.Error(t, err)
}

func TestDataDirDefaultsToDeploymentVolume(t *testing.T) {
	t.Setenv("KWATCH_DATA_DIR", "")
	require.Equal(t, defaultDataDir, dataDir())
	t.Setenv("KWATCH_DATA_DIR", "/x")
	require.Equal(t, "/x", dataDir())
}
