package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func watchedKind(
	kinds []kube.WatchedKind, kind string,
) (kube.WatchedKind, bool) {
	for _, k := range kinds {
		if k.Kind == kind {
			return k, true
		}
	}
	return kube.WatchedKind{}, false
}

func TestSourceCoverageNamesWatchesAndWhenTheySynced(t *testing.T) {
	client := fake.NewSimpleClientset(pod("p1"))
	forbid(client, "secrets")
	src, ctx, stop := runSource(t, kube.SourceConfig{
		Client: client, Now: fixedTime, Submit: noSubmit,
		OptionalSyncTimeout: time.Hour,
	})
	defer stop()
	assert.True(t, src.SyncedAt().IsZero(), "not synced yet")

	require.True(t, src.WaitForSync(ctx))

	assert.True(t, fixedTime().Equal(src.SyncedAt()))
	kinds := src.WatchedKinds()
	pods, ok := watchedKind(kinds, "pods")
	require.True(t, ok)
	assert.Equal(t, kube.WatchedKind{Kind: "pods", Mode: "full",
		Required: true, Synced: true}, pods)
	secrets, ok := watchedKind(kinds, "secrets")
	require.True(t, ok)
	assert.Equal(t, "hashed", secrets.Mode)
	assert.False(t, secrets.Synced, "the API refused the list")
}
