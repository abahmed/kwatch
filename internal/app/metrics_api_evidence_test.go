package app

import (
	"context"
	"testing"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/require"
)

func TestMetricsAPIInspectorReportsMissingRegistration(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			apiServiceGVR: "APIServiceList",
		},
	)
	inspector := newMetricsAPIInspector(
		context.Background(), dynamicClient, fake.NewSimpleClientset(),
	)

	evidence := inspector()
	require.True(t, evidence.Observed)
	require.False(t, evidence.Registered)
}

func TestMetricsAPIInspectorReportsServiceEndpointHealth(t *testing.T) {
	apiService := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiregistration.k8s.io/v1",
		"kind":       "APIService",
		"metadata": map[string]interface{}{
			"name": "v1beta1.metrics.k8s.io",
		},
		"spec": map[string]interface{}{
			"service": map[string]interface{}{
				"namespace": "monitoring",
				"name":      "metrics-server",
			},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "Available",
					"status": "True",
				},
			},
		},
	}}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			apiServiceGVR: "APIServiceList",
		}, apiService,
	)
	client := fake.NewSimpleClientset(&discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "monitoring",
			Name:      "metrics-server-1",
			Labels: map[string]string{
				discoveryv1.LabelServiceName: "metrics-server",
			},
		},
		Endpoints: []discoveryv1.Endpoint{{
			Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)},
		}},
	})
	inspector := newMetricsAPIInspector(
		context.Background(), dynamicClient, client,
	)

	evidence := inspector()
	require.True(t, evidence.Observed)
	require.True(t, evidence.Registered)
	require.True(t, evidence.Available)
	require.True(t, evidence.EndpointsSeen)
	require.Equal(t, 1, evidence.ReadyEndpoints)
	require.Equal(t, "metrics-server", evidence.Service.Name)
}

func boolPtr(value bool) *bool { return &value }
