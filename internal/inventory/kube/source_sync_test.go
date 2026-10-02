package kube_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// forbid makes every list of resource fail as RBAC would.
func forbid(client *fake.Clientset, resource string) {
	client.PrependReactor("list", resource,
		func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(
				schema.GroupResource{Resource: resource}, "",
				nil)
		})
}

func runSource(
	t *testing.T, cfg kube.SourceConfig,
) (*kube.Source, context.Context, func()) {
	t.Helper()
	src, err := kube.NewSource(cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()
	return src, ctx, func() { cancel(); <-done }
}

func noSubmit(context.Context, ...inventory.Observation) {}

func statusFor(
	statuses []kube.SourceStatus, resource string,
) (kube.SourceStatus, bool) {
	for _, s := range statuses {
		if s.Resource == resource {
			return s, true
		}
	}
	return kube.SourceStatus{}, false
}

func TestSourceForbiddenOptionalKindDoesNotBlockSync(t *testing.T) {
	client := fake.NewSimpleClientset(pod("p1"))
	forbid(client, "secrets")
	src, ctx, stop := runSource(t, kube.SourceConfig{
		Client: client, Now: fixedTime, Submit: noSubmit,
		OptionalSyncTimeout: time.Hour,
	})
	defer stop()

	require.True(t, src.WaitForSync(ctx))

	assert.True(t, src.Synced(kube.KindPod))
	assert.False(t, src.Synced(kube.KindSecret))
	status, ok := statusFor(src.Unavailable(), "secrets")
	require.True(t, ok)
	assert.Equal(t, kube.ReasonPermissionDenied, status.Reason)
	assert.False(t, status.Required)
	_, ok = statusFor(src.Unavailable(), "pods")
	assert.False(t, ok)
}

func TestSourceForbiddenRequiredKindBlocksSync(t *testing.T) {
	client := fake.NewSimpleClientset()
	forbid(client, "pods")
	src, _, stop := runSource(t, kube.SourceConfig{
		Client: client, Now: fixedTime, Submit: noSubmit,
		OptionalSyncTimeout: time.Millisecond,
	})
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(),
		500*time.Millisecond)
	defer cancel()

	require.False(t, src.WaitForSync(ctx), "pods are required")

	status, ok := statusFor(src.Unavailable(), "pods")
	require.True(t, ok)
	assert.True(t, status.Required)
	assert.Equal(t, kube.ReasonPermissionDenied, status.Reason)
}

// A kind that keeps failing with a transient error is waited for only up
// to OptionalSyncTimeout, then reported and skipped.
func TestSourceOptionalSyncTimesOut(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "configmaps",
		func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewInternalError(
				assert.AnError)
		})
	src, ctx, stop := runSource(t, kube.SourceConfig{
		Client: client, Now: fixedTime, Submit: noSubmit,
		OptionalSyncTimeout: 50 * time.Millisecond,
	})
	defer stop()

	require.True(t, src.WaitForSync(ctx))

	status, ok := statusFor(src.Unavailable(), "configmaps")
	require.True(t, ok)
	assert.Equal(t, kube.ReasonSyncFailed, status.Reason)
}

// Handlers submit with the source's lifecycle context: a pipeline that
// is full (Submit blocks until its ctx ends) cannot hold shutdown.
func TestSourceHandlersStopBlockingOnShutdown(t *testing.T) {
	client := fake.NewSimpleClientset(pod("p1"), node("n1"))
	entered := make(chan struct{}, 1)
	src, err := kube.NewSource(kube.SourceConfig{
		Client: client, Now: fixedTime,
		Submit: func(ctx context.Context, _ ...inventory.Observation) {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-ctx.Done()
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()
	<-entered

	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("source did not stop while a handler was blocked")
	}
}

// blockKind returns a Submit that blocks the first observation of kind
// until release closes, and a channel that fires when it blocks.
func blockKind(
	kind inventory.Kind, release <-chan struct{},
) (kube.Submit, <-chan struct{}) {
	entered := make(chan struct{}, 1)
	return func(ctx context.Context, obs ...inventory.Observation) {
		for _, o := range obs {
			if o.Entity.Kind != kind {
				continue
			}
			select {
			case entered <- struct{}{}:
			default:
				return
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return
		}
	}, entered
}

// A kind is synced once its handler has submitted the initial list, not
// as soon as the informer's store holds it: the model is fed only
// through Submit.
func TestSourceSyncedWaitsForHandlerToSubmitInitialList(t *testing.T) {
	release := make(chan struct{})
	submit, entered := blockKind(kube.KindPod, release)
	src, ctx, stop := runSource(t, kube.SourceConfig{
		Client: fake.NewSimpleClientset(pod("p1"), node("n1")),
		Now:    fixedTime, Submit: submit, OptionalSyncTimeout: time.Hour,
	})
	defer stop()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("pod handler never ran")
	}

	assert.False(t, src.Synced(kube.KindPod),
		"pod handler has not finished the initial list")
	assert.False(t, src.Synced(kube.KindContainer))
	status, ok := statusFor(src.Unavailable(), "pods")
	require.True(t, ok)
	assert.Equal(t, kube.ReasonSyncPending, status.Reason)

	close(release)
	require.True(t, src.WaitForSync(ctx))
	assert.True(t, src.Synced(kube.KindPod))
}
