package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
)

func leaseWithTransitions(n *int32) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kwatch-test-leader", Namespace: "kwatch",
		},
		Spec: coordinationv1.LeaseSpec{LeaseTransitions: n},
	}
}

func epochDeps(objects ...*coordinationv1.Lease) *serverDeps {
	clientset := fake.NewSimpleClientset()
	for _, lease := range objects {
		_, _ = clientset.CoordinationV1().Leases("kwatch").Create(
			context.Background(), lease, metav1.CreateOptions{})
	}
	return &serverDeps{clients: client.ClientSet{
		Kubernetes: clientset, Clock: clock.RealClock{},
	}}
}

func TestLeaseEpochFollowsLeaseTransitions(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "kwatch")
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-test-leader")
	tests := map[string]struct {
		transitions *int32
		want        uint64
	}{
		"never transitioned": {nil, 1},
		"third holder":       {ptr.To(int32(2)), 3},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			deps := epochDeps(leaseWithTransitions(tc.transitions))

			got, err := leaseEpoch(context.Background(), deps)

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLeaseEpochFailsWithoutLease(t *testing.T) {
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-test-leader")

	_, err := leaseEpoch(context.Background(), epochDeps())

	require.ErrorContains(t, err, "read lease epoch")
}

func TestOpenStoreClaimsEpochFromLease(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KWATCH_DATA_DIR", dir)
	t.Setenv("POD_NAMESPACE", "kwatch")
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-test-leader")
	deps := epochDeps(leaseWithTransitions(ptr.To(int32(4))))

	s, err := openStore(context.Background(), deps)

	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, "state.db"))
	closeStore(s)
	closeStore(s)
}

func TestOpenStoreFencesOlderLeaseHolder(t *testing.T) {
	t.Setenv("KWATCH_DATA_DIR", t.TempDir())
	t.Setenv("POD_NAMESPACE", "kwatch")
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-test-leader")
	newer := epochDeps(leaseWithTransitions(ptr.To(int32(5))))
	s, err := openStore(context.Background(), newer)
	require.NoError(t, err)
	closeStore(s)
	older := epochDeps(leaseWithTransitions(ptr.To(int32(1))))

	_, err = openStore(context.Background(), older)

	require.Error(t, err, "an older epoch must not claim the store")
}

func TestOpenStoreFailsWithoutLease(t *testing.T) {
	t.Setenv("KWATCH_DATA_DIR", t.TempDir())
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-test-leader")

	_, err := openStore(context.Background(), epochDeps())

	require.Error(t, err)
}

func TestOpenStoreFailsWhenDataDirIsAFile(t *testing.T) {
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
