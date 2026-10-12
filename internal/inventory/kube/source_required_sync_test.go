package kube_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A slow optional kind must not hold the startup: the required kinds are
// enough, and the optional wait still runs in the background.
func TestSourceRequiredSyncDoesNotWaitForOptionalKinds(t *testing.T) {
	client := fake.NewSimpleClientset(pod("p1"))
	client.PrependReactor("list", "configmaps",
		func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewInternalError(assert.AnError)
		})
	src, _, stop := runSource(t, kube.SourceConfig{
		Client: client, Now: fixedTime, Submit: noSubmit,
		OptionalSyncTimeout: time.Hour,
	})
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.True(t, src.WaitForRequiredSync(ctx))

	assert.True(t, src.Synced(kube.KindPod))
	assert.False(t, src.Synced(kube.KindConfigMap))
}

func TestSourceRequiredSyncStopsWithContext(t *testing.T) {
	client := fake.NewSimpleClientset()
	forbid(client, "pods")
	src, _, stop := runSource(t, kube.SourceConfig{
		Client: client, Now: fixedTime, Submit: noSubmit,
	})
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(),
		200*time.Millisecond)
	defer cancel()

	assert.False(t, src.WaitForRequiredSync(ctx), "pods are required")
}
