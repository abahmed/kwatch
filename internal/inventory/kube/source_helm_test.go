package kube_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// releaseSecret is the metadata of a Helm release Secret.
func releaseSecret() *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace, Name: "sh.helm.release.v1.api.v4",
			Labels: map[string]string{"owner": "helm", "name": "api",
				"status": "failed", "version": "4"},
			CreationTimestamp: metav1.NewTime(fixedTime()),
		},
	}
}

func TestSourceWatchesHelmReleaseLabels(t *testing.T) {
	scheme := metadatafake.NewTestScheme()
	require.NoError(t, metav1.AddMetaToScheme(scheme))
	meta := metadatafake.NewSimpleMetadataClient(scheme, releaseSecret())
	observations := make(chan inventory.Observation, 256)
	src, err := kube.NewSource(kube.SourceConfig{
		Client:   fake.NewSimpleClientset(pod("p1"), node("n1")),
		Metadata: meta, Resync: time.Hour, Now: fixedTime,
		Submit: func(_ context.Context, f ...inventory.Observation) {
			for _, observation := range f {
				observations <- observation
			}
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()
	require.True(t, src.WaitForSync(ctx))
	assert.True(t, src.Synced(kube.KindHelmRelease))
	id := kube.HelmReleaseID(testNamespace, "api")
	for observation := range observations {
		if observation.Entity == id {
			assert.Equal(t, "failed",
				observation.Attributes[kube.AttrHelmStatus].AsText())
			break
		}
	}
	cancel()
	<-done
}

func TestSourceWithoutMetadataLeavesHelmUnsynced(t *testing.T) {
	src, err := kube.NewSource(kube.SourceConfig{
		Client: fake.NewSimpleClientset(), Resync: time.Hour,
		Now:    fixedTime,
		Submit: func(context.Context, ...inventory.Observation) {},
	})
	require.NoError(t, err)
	assert.False(t, src.Synced(kube.KindHelmRelease))
}
