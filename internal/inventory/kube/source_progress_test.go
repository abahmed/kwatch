package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestSourceSyncProgressCountsKindsAndListedObjects(t *testing.T) {
	client := fake.NewSimpleClientset(pod("p1"), pod("p2"))
	forbid(client, "secrets")
	src, err := kube.NewSource(kube.SourceConfig{
		Client: client, Now: fixedTime, Submit: noSubmit,
		OptionalSyncTimeout: time.Hour,
	})
	require.NoError(t, err)
	before := src.SyncProgress()
	assert.Positive(t, before.Kinds)
	assert.Zero(t, before.Synced)
	assert.Zero(t, before.Objects)
	assert.False(t, before.Done)

	src, ctx, stop := runSource(t, kube.SourceConfig{
		Client: client, Now: fixedTime, Submit: noSubmit,
		OptionalSyncTimeout: time.Hour,
	})
	defer stop()
	require.True(t, src.WaitForSync(ctx))

	got := src.SyncProgress()
	assert.True(t, got.Done)
	assert.Equal(t, got.Kinds, got.Settled)
	assert.Equal(t, got.Kinds-1, got.Synced, "secrets are refused")
	assert.Equal(t, got.Required, got.RequiredSynced)
	assert.GreaterOrEqual(t, got.Objects, int64(2), "both pods listed")
}
