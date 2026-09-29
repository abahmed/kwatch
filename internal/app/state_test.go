package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/knowledge/store"
	"github.com/abahmed/kwatch/internal/model"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"),
		store.Options{Now: func() time.Time {
			return time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
		}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.NoError(t, s.Claim(1))
	return s
}

func kubeSystem(uid string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "kube-system", UID: types.UID(uid),
	}}
}

func TestDiskStateClusterIDDerivesFromNamespaceUID(t *testing.T) {
	ctx := context.Background()
	first := diskState{store: openTestStore(t),
		client: fake.NewSimpleClientset(kubeSystem("uid-1"))}
	second := diskState{store: openTestStore(t),
		client: fake.NewSimpleClientset(kubeSystem("uid-1"))}

	a, err := first.EnsureClusterID(ctx)
	require.NoError(t, err)
	b, err := second.EnsureClusterID(ctx)
	require.NoError(t, err)
	again, err := first.EnsureClusterID(ctx)
	require.NoError(t, err)

	require.Equal(t, a, b, "a reinstall keeps the cluster identity")
	require.Equal(t, a, again)
	require.Equal(t, clusterIDFromUID("uid-1"), a)
	require.NotContains(t, a, "uid-1")
}

func TestDiskStateClusterIDFallsBackToRandomID(t *testing.T) {
	ctx := context.Background()
	tests := map[string]diskState{
		"no client": {store: openTestStore(t)},
		"no namespace": {store: openTestStore(t),
			client: fake.NewSimpleClientset()},
		"empty uid": {store: openTestStore(t),
			client: fake.NewSimpleClientset(kubeSystem(""))},
	}
	for name, disk := range tests {
		t.Run(name, func(t *testing.T) {
			id, err := disk.EnsureClusterID(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, id)
			stored, err := disk.EnsureClusterID(ctx)
			require.NoError(t, err)
			require.Equal(t, id, stored)
		})
	}
}

func TestDiskStateClusterIDFailsOnClosedStore(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.Close())

	_, err := diskState{store: s}.EnsureClusterID(context.Background())

	require.Error(t, err)
}

func TestClusterIDFromUIDIsVersion4UUID(t *testing.T) {
	id := clusterIDFromUID("x")

	require.Len(t, id, 36)
	require.Equal(t, byte('4'), id[14])
	require.NotEqual(t, id, clusterIDFromUID("y"))
}

func TestDiskStateInitializationLifecycle(t *testing.T) {
	ctx := context.Background()
	disk := diskState{store: openTestStore(t)}

	first, err := disk.IsFirstRun(ctx)
	require.NoError(t, err)
	require.True(t, first)
	version, err := disk.GetStoredVersion(ctx)
	require.NoError(t, err)
	require.Empty(t, version)

	require.NoError(t, disk.MarkAsInitialized(ctx, "cid", "v1.2.3"))

	first, err = disk.IsFirstRun(ctx)
	require.NoError(t, err)
	require.False(t, first)
	version, _ = disk.GetStoredVersion(ctx)
	require.Equal(t, "v1.2.3", version)
	id, _ := disk.EnsureClusterID(ctx)
	require.Equal(t, "cid", id)
}

func TestDiskStateLastSeenAndTelemetryTimestamps(t *testing.T) {
	ctx := context.Background()
	disk := diskState{store: openTestStore(t)}
	at := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)

	zero, err := disk.GetLastSeen(ctx)
	require.NoError(t, err)
	require.True(t, zero.IsZero())
	require.NoError(t, disk.SetLastSeen(ctx, at))
	require.NoError(t, disk.SetTelemetryLastSent(ctx, at.Add(time.Hour)))

	seen, _ := disk.GetLastSeen(ctx)
	sent, err := disk.GetTelemetryLastSent(ctx)
	require.NoError(t, err)
	require.True(t, seen.Equal(at))
	require.True(t, sent.Equal(at.Add(time.Hour)))
}

func TestDiskStateRuntimeSessionRoundTrips(t *testing.T) {
	ctx := context.Background()
	disk := diskState{store: openTestStore(t)}

	empty, err := disk.GetRuntimeSession(ctx)
	require.NoError(t, err)
	require.Empty(t, empty.PodName)
	require.NoError(t, disk.SaveRuntimeSession(ctx,
		model.RuntimeSession{PodName: "kwatch-0", NodeName: "n1"}))

	got, err := disk.GetRuntimeSession(ctx)
	require.NoError(t, err)
	require.Equal(t, "kwatch-0", got.PodName)
	require.Equal(t, "n1", got.NodeName)
}

func TestDiskStateStartupAnnouncementClaimsOncePerKey(t *testing.T) {
	ctx := context.Background()
	disk := diskState{store: openTestStore(t)}

	first, err := disk.ClaimStartupAnnouncement(ctx, "a")
	require.NoError(t, err)
	repeat, _ := disk.ClaimStartupAnnouncement(ctx, "a")
	next, _ := disk.ClaimStartupAnnouncement(ctx, "b")

	require.True(t, first)
	require.False(t, repeat, "a quick restart must not repeat the message")
	require.True(t, next)
}

func TestDiskStateNotifiedVersionRoundTrips(t *testing.T) {
	ctx := context.Background()
	disk := diskState{store: openTestStore(t)}

	require.Empty(t, disk.GetNotifiedVersion(ctx))
	require.NoError(t, disk.SetNotifiedVersion(ctx, "v9"))

	require.Equal(t, "v9", disk.GetNotifiedVersion(ctx))
}

func TestDiskStateReportsWriteFailuresAfterClose(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	disk := diskState{store: s}
	require.NoError(t, s.Close())

	require.Error(t, disk.MarkAsInitialized(ctx, "c", "v"))
	require.Error(t, disk.SetLastSeen(ctx, time.Now()))
	_, err := disk.ClaimStartupAnnouncement(ctx, "k")
	require.Error(t, err)
	_, err = disk.IsFirstRun(ctx)
	require.Error(t, err)
}
