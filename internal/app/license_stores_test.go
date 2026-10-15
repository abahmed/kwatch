package app

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/abahmed/kwatch/internal/license"
)

func TestInstanceStoreTouchesOnlyItsSecret(t *testing.T) {
	client := fake.NewSimpleClientset(emptyInstanceSecret("kwatch"))
	store := kubeInstanceStore{client: client, namespace: "kwatch"}
	ctx := context.Background()

	// The shipped empty Secret has no ed25519 key: it reads as missing.
	_, ok, err := store.Load(ctx)
	require.NoError(t, err)
	require.False(t, ok)

	key, err := license.EnsureInstanceKey(ctx, store)
	require.NoError(t, err)
	again, ok, err := store.Load(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, key.Equal(again), "the key persists")

	secret, err := client.CoreV1().Secrets("kwatch").Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, key.Seed(), secret.Data["ed25519"])
	require.Len(t, secret.Data["ed25519"], ed25519.SeedSize)

	// A malformed Secret reads as no key and is replaced by Update.
	secret.Data["ed25519"] = []byte("short")
	_, err = client.CoreV1().Secrets("kwatch").Update(
		ctx, secret, metav1.UpdateOptions{})
	require.NoError(t, err)
	fresh, err := license.EnsureInstanceKey(ctx, store)
	require.NoError(t, err)
	require.False(t, key.Equal(fresh))

	for _, action := range client.Actions() {
		require.Equal(t, "secrets", action.GetResource().Resource)
		switch a := action.(type) {
		case k8stesting.GetAction:
			require.Equal(t, licenseInstanceSecret, a.GetName())
		case k8stesting.UpdateAction:
			require.Equal(t, licenseInstanceSecret,
				a.GetObject().(*corev1.Secret).Name)
		default:
			t.Fatalf("unexpected action %v", action)
		}
		require.Contains(t, []string{"get", "update"},
			action.GetVerb())
	}
}

func TestInstanceStoreNeverCreatesAMissingSecret(t *testing.T) {
	client := fake.NewSimpleClientset()
	store := kubeInstanceStore{client: client, namespace: "kwatch"}
	ctx := context.Background()

	_, ok, err := store.Load(ctx)
	require.NoError(t, err)
	require.False(t, ok)

	_, err = license.EnsureInstanceKey(ctx, store)
	require.Error(t, err)
	require.Contains(t, err.Error(), "kwatch/"+licenseInstanceSecret)
	require.Contains(t, err.Error(), "get and update")
	require.Contains(t, err.Error(), "never creates it")

	for _, action := range client.Actions() {
		require.NotEqual(t, "create", action.GetVerb())
	}
	list, err := client.CoreV1().Secrets("kwatch").List(
		ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, list.Items)
}

func TestLeaseStoreKeepsTheLeaseAndTheClockGuard(t *testing.T) {
	store := boltLeaseStore{disk: diskState{store: openTestStore(t)}}
	lease, guard, err := store.LoadLease()
	require.NoError(t, err)
	require.Empty(t, lease)
	require.True(t, guard.MaxSeen.IsZero())

	seen := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	require.NoError(t, store.SaveLease("KWL1-token",
		license.ClockGuard{MaxSeen: seen}))

	lease, guard, err = store.LoadLease()
	require.NoError(t, err)
	require.Equal(t, "KWL1-token", lease)
	require.True(t, seen.Equal(guard.MaxSeen))
	require.Equal(t, "license.lease", stateLicenseLease)
	require.Equal(t, "license.clock", stateLicenseClock)
}

func TestKubeSystemUIDIsReadLive(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system", UID: types.UID("uid-1"),
		},
	})
	read := kubeSystemUID(client)
	uid, err := read(context.Background())
	require.NoError(t, err)
	require.Equal(t, "uid-1", uid)

	require.NoError(t, client.CoreV1().Namespaces().Delete(
		context.Background(), "kube-system", metav1.DeleteOptions{}))
	_, err = read(context.Background())
	require.Error(t, err, "a deleted namespace is not served from a cache")
}

func TestNodeCountUsesTheRemainingItemCount(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "nodes",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			list := &corev1.NodeList{Items: []corev1.Node{{}}}
			list.RemainingItemCount = ptr.To(int64(6))
			return true, list, nil
		})
	require.Equal(t, 7, nodeCount(client)(context.Background()))
	require.Equal(t, 0, nodeCount(fake.NewSimpleClientset())(
		context.Background()))
}
