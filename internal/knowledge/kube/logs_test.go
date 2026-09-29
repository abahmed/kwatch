package kube_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestLogReaderExcerptRejectsBadInput(t *testing.T) {
	id := kube.ContainerID(testNamespace, "p", "app")
	assert.Nil(t, kube.LogReader{}.Excerpt(context.Background(), id))
	r := kube.LogReader{Client: fake.NewSimpleClientset()}
	bad := id
	bad.Name = "noslash"
	assert.Nil(t, r.Excerpt(context.Background(), bad))
}

func TestLogReaderExcerptReadsFakeLogs(t *testing.T) {
	r := kube.LogReader{Client: fake.NewSimpleClientset(pod("p"))}
	id := kube.ContainerID(testNamespace, "p", "app")
	// The fake clientset serves the constant body "fake logs".
	got := r.Excerpt(context.Background(), id)
	assert.Equal(t, []string{"fake logs"}, got)
}
