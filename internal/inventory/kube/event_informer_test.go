package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func TestWarningEventInformerListsOnlyWarnings(t *testing.T) {
	client := fake.NewClientset()
	factory := informers.NewSharedInformerFactory(client, 0)
	informer := warningEventInformer(factory)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	factory.Start(ctx.Done())
	require.True(t, cache.WaitForCacheSync(ctx.Done(), informer.HasSynced))
	cancel()
	factory.Shutdown()

	var selectors []string
	for _, action := range client.Actions() {
		if list, ok := action.(k8stesting.ListAction); ok &&
			action.GetResource().Resource == "events" {
			selectors = append(selectors,
				list.GetListRestrictions().Fields.String())
		}
	}
	assert.Equal(t, []string{"type=Warning"}, selectors)
}

func TestWarningEventInformerIsSharedByFactory(t *testing.T) {
	factory := informers.NewSharedInformerFactory(fake.NewClientset(), 0)

	assert.Same(t, warningEventInformer(factory),
		warningEventInformer(factory))
}
