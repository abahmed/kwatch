package kube_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestNewSourceRejectsIncompleteConfig(t *testing.T) {
	_, err := kube.NewSource(kube.SourceConfig{})
	assert.Error(t, err)
}

func TestSourceStreamsObservationsAndSecretsAreHashed(t *testing.T) {
	warning := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: testNamespace},
		Type:       corev1.EventTypeWarning, Reason: "Failed",
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Namespace: testNamespace, Name: "p1",
		},
	}
	client := fake.NewSimpleClientset(
		pod("p1"), node("n1"), secret("s1"), warning)
	observations := make(chan inventory.Observation, 256)
	src, err := kube.NewSource(kube.SourceConfig{
		Client: client, Resync: time.Hour, Now: fixedTime,
		Submit: func(_ context.Context, f ...inventory.Observation) {
			for _, observation := range f {
				observations <- observation
			}
		},
	})
	require.NoError(t, err)
	assert.False(t, src.Synced(kube.KindPod))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()
	require.True(t, src.WaitForSync(ctx))
	assert.True(t, src.Synced(kube.KindPod))
	assert.True(t, src.Synced(kube.KindContainer))
	assert.False(t, src.Synced(inventory.Kind("unknown")))

	var sawPod, sawNote bool
	for !(sawPod && sawNote) {
		observation := <-observations
		if observation.Entity == inventory.CoreID(
			kube.KindPod, testNamespace, "p1") &&
			observation.Kind == inventory.Observed {
			sawPod = true
		}
		if observation.Kind == inventory.Noted &&
			observation.Note.Reason == "Failed" {
			sawNote = true
		}
	}
	cancel()
	<-done
}

func TestSourceWaitForSyncStopsWithContext(t *testing.T) {
	src, err := kube.NewSource(kube.SourceConfig{
		Client: fake.NewSimpleClientset(), Now: fixedTime,
		Submit: func(context.Context, ...inventory.Observation) {},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, src.WaitForSync(ctx))
}

func TestSourceWithSecretsDisabledDoesNotWatchThem(t *testing.T) {
	client := fake.NewSimpleClientset(pod("p1"), node("n1"), secret("s1"))
	var secrets atomic.Int32
	src, err := kube.NewSource(kube.SourceConfig{
		Client: client, Resync: time.Hour, Now: fixedTime,
		DisableSecrets: true,
		Submit: func(_ context.Context, f ...inventory.Observation) {
			for _, observation := range f {
				if observation.Entity.Kind == kube.KindSecret {
					secrets.Add(1)
				}
			}
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()
	require.True(t, src.WaitForSync(ctx))

	assert.False(t, src.Synced(kube.KindSecret))
	assert.False(t, src.Verifiable(kube.KindSecret),
		"checks that need Secrets cannot verify")
	assert.True(t, src.Verifiable(kube.KindConfigMap))
	assert.Equal(t, []kube.SourceStatus{{
		Resource: "secrets", Reason: kube.ReasonDisabledByConfig,
	}}, src.Disabled())
	for _, s := range src.Unavailable() {
		assert.NotEqual(t, "secrets", s.Resource,
			"a disabled kind is not a failure")
	}
	for _, action := range client.Actions() {
		assert.NotEqual(t, "secrets", action.GetResource().Resource,
			"no Secret list or watch is sent")
	}
	cancel()
	<-done
	assert.Zero(t, secrets.Load())
}

// A panicking handler is contained: the informer keeps delivering.
func TestSourceContainsHandlerPanics(t *testing.T) {
	client := fake.NewSimpleClientset(pod("p1"), node("n1"))
	var calls atomic.Int32
	src, err := kube.NewSource(kube.SourceConfig{
		Client: client, Resync: time.Hour, Now: fixedTime,
		Submit: func(context.Context, ...inventory.Observation) {
			if calls.Add(1) == 1 {
				panic("translator bug")
			}
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()
	require.True(t, src.WaitForSync(ctx))
	_, err = client.CoreV1().Pods(testNamespace).Create(ctx, pod("p2"),
		metav1.CreateOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return calls.Load() >= 3 },
		10*time.Second, 10*time.Millisecond)
	cancel()
	<-done
}
