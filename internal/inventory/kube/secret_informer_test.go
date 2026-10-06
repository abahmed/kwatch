package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestSecretInformerDoesNotWatchHelmReleaseSecrets(t *testing.T) {
	client := fake.NewSimpleClientset()
	selectors := make(chan fields.Selector, 4)
	record := func(action clienttesting.Action) {
		if list, ok := action.(clienttesting.ListAction); ok {
			selectors <- list.GetListRestrictions().Fields
		}
	}
	client.PrependReactor("list", "secrets",
		func(a clienttesting.Action) (bool, runtime.Object, error) {
			record(a)
			return false, nil, nil
		})
	factory := informers.NewSharedInformerFactory(client, time.Hour)
	informer := secretInformer(factory)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go informer.Run(ctx.Done())
	selector := <-selectors

	assert.False(t, selector.Matches(fields.Set{
		"type": helmReleaseSecretType}), "a Helm release is not listed")
	assert.True(t, selector.Matches(fields.Set{"type": "Opaque"}))
	assert.True(t, selector.Matches(fields.Set{
		"type": "kubernetes.io/tls"}))
	require.NotNil(t, informer)
}
