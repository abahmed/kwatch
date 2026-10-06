package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

// Each autoscaler reason has its own informer; one reaches the handler.
func TestScaleEventFactoriesDeliverEvents(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: "d"},
		Type:       corev1.EventTypeNormal, Reason: ReasonTriggeredScaleUp,
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "p"},
	})
	got := make(chan string, 8)
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) { got <- obj.(*corev1.Event).Reason },
	}
	factories, err := scaleEventFactories(SourceConfig{Client: client},
		handler)
	require.NoError(t, err)
	assert.Len(t, factories, len(autoscalerNormalReasons))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, f := range factories {
		f.Start(ctx.Done())
	}
	select {
	case reason := <-got:
		assert.Equal(t, ReasonTriggeredScaleUp, reason)
	case <-time.After(5 * time.Second):
		t.Fatal("no event delivered")
	}
	cancel()
	for _, f := range factories {
		f.Shutdown()
	}
}
