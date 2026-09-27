package network

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
)

func TestServiceDegradationRequiresSustainedBackendLoss(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app": "api"},
			ClusterIP: "10.0.0.1",
		},
	}
	pods := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, pod := range []*corev1.Pod{
		backendPod("one", "node-a", false),
		backendPod("two", "node-b", true),
	} {
		pod.Namespace = "apps"
		pod.Labels = map[string]string{"app": "api"}
		require.NoError(t, pods.Add(pod))
	}
	slices := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	require.NoError(t, slices.Add(&discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-1", Namespace: "apps",
			Labels: map[string]string{
				"kubernetes.io/service-name": "api",
			},
		},
		Endpoints: []discoveryv1.Endpoint{{
			Conditions: discoveryv1.EndpointConditions{
				Ready: boolPtr(true),
			},
		}},
	}))
	sink := &networkSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, func() time.Time { return now },
	)
	require.NoError(t, runtime.ConfigureSources(Sources{
		Pods:          corev1lister.NewPodLister(pods),
		EndpointSlice: discoveryv1lister.NewEndpointSliceLister(slices),
	}))
	require.NoError(t, runtime.ProcessServiceObject(svc, false))
	assert.Empty(t, sink.findings)
	now = now.Add(degradedServiceSustain)
	require.NoError(t, runtime.ProcessServiceObject(svc, false))
	require.Len(t, sink.findings, 1)
	assert.Equal(t, constant.ReasonServiceBackendsDegraded,
		sink.findings[0].Reason)
}
