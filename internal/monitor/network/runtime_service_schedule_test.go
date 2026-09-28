package network

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
)

func TestServiceOutageRechecksAtSustainDeadline(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "api"},
			ClusterIP: "10.0.0.1",
		},
	}
	indexers := cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	}
	pods := cache.NewIndexer(cache.MetaNamespaceKeyFunc, indexers)
	slices := cache.NewIndexer(cache.MetaNamespaceKeyFunc, indexers)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-1", Namespace: "apps",
			Labels: map[string]string{"app": "api"},
		},
		Spec: corev1.PodSpec{},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
			Type: corev1.PodReady, Status: corev1.ConditionTrue,
		}}},
	}
	pods.Add(pod)
	var rechecks []time.Duration
	sink := &networkSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, func() time.Time { return now },
	)
	require.NoError(t, runtime.ConfigureSources(Sources{
		Pods:          corev1lister.NewPodLister(pods),
		EndpointSlice: discoveryv1lister.NewEndpointSliceLister(slices),
		RequeueService: func(key string, delay time.Duration) {
			assert.Equal(t, "apps/api", key)
			rechecks = append(rechecks, delay)
		},
	}))
	require.NoError(t, runtime.ProcessServiceObject(svc, false))
	assert.Empty(t, sink.findings)
	assert.Equal(t, []time.Duration{serviceSustain}, rechecks)
	now = now.Add(serviceSustain)
	require.NoError(t, runtime.ProcessServiceObject(svc, false))
	require.Len(t, sink.findings, 1)
	assert.Equal(t, constant.ReasonServiceNoEndpoints,
		sink.findings[0].Reason)
	assert.Len(t, rechecks, 1)
}
