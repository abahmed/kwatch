package app

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/license"
)

func TestLicenseClusterIDIsDerivedFromTheKubeSystemUID(t *testing.T) {
	w := newLicenseWorld(t)
	deps := licensedDeps(t, w, w.key(license.PlanPro, time.Hour),
		feature.CommunityEntitlements())
	l := &licenseSession{
		deps: deps, now: w.clock.Now, tracker: &renewTracker{now: w.clock.Now},
	}

	cfg := l.managerConfig(deps.license)

	require.Empty(t, cfg.ClusterUUID, "no fixed or stored id is configured")
	require.NotNil(t, cfg.ClusterUUIDFrom)
	require.Equal(t, clusterIDFromUID("uid-kube-system"),
		cfg.ClusterUUIDFrom("uid-kube-system"))
}

func TestLicenseStaysPendingWithoutTheKubeSystemUID(t *testing.T) {
	w := newLicenseWorld(t)
	key := w.key(license.PlanPro, 300*24*time.Hour)
	deps := licensedDeps(t, w, key, feature.CommunityEntitlements())
	require.NoError(t, deps.clients.Kubernetes.CoreV1().Namespaces().Delete(
		context.Background(), "kube-system", metav1.DeleteOptions{}))
	deps.license.initialSync = 200 * time.Millisecond

	done, stop := runSession(t, deps)
	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond)
	stop()
	require.NoError(t, awaitSession(t, done))

	require.Zero(t, w.total(), "no id may be guessed and activated")
	require.False(t, deps.editionRestart.Load())
	require.Contains(t, scrapeMetrics(t),
		`kwatch_license_state{state="pending"} 1`)
}

func TestLicenseDeactivationMarkerDropsTheRunningLease(t *testing.T) {
	w := newLicenseWorld(t)
	deps := activated(t, w, w.key(license.PlanPro, 300*24*time.Hour))
	deps.license.processStart = w.clock.Now().Add(-time.Hour)
	done, _ := runSession(t, deps)
	require.Eventually(t, deps.healthServer.Ready, 15*time.Second,
		5*time.Millisecond)

	// What `kwatch license deactivate` writes from its own process.
	store := kubeInstanceStore{
		client: deps.clients.Kubernetes, namespace: kubeclient.GetNamespace(),
	}
	require.NoError(t, store.MarkDeactivated(
		context.Background(), w.clock.Now().Add(time.Second)))

	require.ErrorIs(t, awaitSession(t, done), errEditionChanged)
	require.Equal(t, feature.CommunityEdition, deps.edition.Load().Edition())
}

func TestDeactivationMarkerRoundTripAndSavePreservesIt(t *testing.T) {
	ctx := context.Background()
	kube := fake.NewSimpleClientset(emptyInstanceSecret("kwatch"))
	store := kubeInstanceStore{client: kube, namespace: "kwatch"}

	at, err := store.DeactivatedAt(ctx)
	require.NoError(t, err)
	require.True(t, at.IsZero())

	stamp := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	require.NoError(t, store.MarkDeactivated(ctx, stamp))
	_, err = license.EnsureInstanceKey(ctx, store)
	require.NoError(t, err)

	at, err = store.DeactivatedAt(ctx)
	require.NoError(t, err)
	require.True(t, stamp.Equal(at), "saving the key kept the marker")
	_, found, err := store.Load(ctx)
	require.NoError(t, err)
	require.True(t, found)
}

func TestLicenseDeactivateCLIWritesTheMarker(t *testing.T) {
	w := newLicenseWorld(t)
	key := w.key(license.PlanPro, 30*24*time.Hour)
	kube := fake.NewSimpleClientset(kubeSystemNS(), emptyInstanceSecret("kwatch"))
	store := kubeInstanceStore{client: kube, namespace: "kwatch"}
	_, err := license.EnsureInstanceKey(context.Background(), store)
	require.NoError(t, err)
	var out, errOut bytes.Buffer

	cli := testCLI(w, key, kube)
	require.Zero(t, cli.deactivate(context.Background(), &out, &errOut),
		errOut.String())

	at, err := store.DeactivatedAt(context.Background())
	require.NoError(t, err)
	require.False(t, at.IsZero(), "marker written")
}
