package kube_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestNewSourceRejectsIncompleteConfig(t *testing.T) {
	_, err := kube.NewSource(kube.SourceConfig{})
	assert.Error(t, err)
}

func TestSourceStreamsFactsAndSecretsAreHashed(t *testing.T) {
	warning := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: testNamespace},
		Type:       corev1.EventTypeWarning, Reason: "Failed",
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Namespace: testNamespace, Name: "p1",
		},
	}
	client := fake.NewSimpleClientset(
		pod("p1"), node("n1"), secret("s1"), warning)
	facts := make(chan knowledge.Fact, 256)
	src, err := kube.NewSource(kube.SourceConfig{
		Client: client, Resync: time.Hour, Now: fixedTime,
		Submit: func(_ context.Context, f ...knowledge.Fact) {
			for _, fact := range f {
				facts <- fact
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
	assert.False(t, src.Synced(knowledge.Kind("unknown")))

	var sawPod, sawNote bool
	for !(sawPod && sawNote) {
		fact := <-facts
		if fact.Entity == knowledge.NewEntityID(
			kube.KindPod, testNamespace, "p1") &&
			fact.Kind == knowledge.Observed {
			sawPod = true
		}
		if fact.Kind == knowledge.Noted && fact.Note.Reason == "Failed" {
			sawNote = true
		}
	}
	cancel()
	<-done
}

func TestSourceWaitForSyncStopsWithContext(t *testing.T) {
	src, err := kube.NewSource(kube.SourceConfig{
		Client: fake.NewSimpleClientset(), Now: fixedTime,
		Submit: func(context.Context, ...knowledge.Fact) {},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, src.WaitForSync(ctx))
}
