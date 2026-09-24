package app

import (
	"context"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

var apiServiceGVR = schema.GroupVersionResource{
	Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices",
}

func newMetricsAPIInspector(
	parent context.Context,
	dynamicClient dynamic.Interface,
	client kubernetes.Interface,
) func() insight.MetricsAPIEvidence {
	return func() insight.MetricsAPIEvidence {
		if dynamicClient == nil || client == nil {
			return insight.MetricsAPIEvidence{}
		}
		ctx, cancel := context.WithTimeout(parent, 2*time.Second)
		defer cancel()
		object, err := dynamicClient.Resource(apiServiceGVR).Get(
			ctx, "v1beta1.metrics.k8s.io", metav1.GetOptions{},
		)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return insight.MetricsAPIEvidence{Observed: true}
			}
			return insight.MetricsAPIEvidence{}
		}
		result := insight.MetricsAPIEvidence{
			Observed: true, Registered: true,
		}
		result.Available, result.Reason = apiServiceAvailability(object.Object)
		namespace, _, _ := unstructured.NestedString(object.Object,
			"spec", "service", "namespace")
		name, _, _ := unstructured.NestedString(object.Object,
			"spec", "service", "name")
		result.Service = model.ObjectRef{
			Kind: "service", Namespace: namespace, Name: name,
		}
		if namespace != "" && name != "" {
			result.ReadyEndpoints, result.EndpointsSeen =
				readyServiceEndpoints(ctx, client, namespace, name)
		}
		return result
	}
}

func apiServiceAvailability(object map[string]interface{}) (bool, string) {
	conditions, _, _ := unstructured.NestedSlice(
		object, "status", "conditions",
	)
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok || condition["type"] != "Available" {
			continue
		}
		available := condition["status"] == "True"
		reason, _ := condition["reason"].(string)
		return available, reason
	}
	return false, "condition_missing"
}

func readyServiceEndpoints(
	ctx context.Context,
	client kubernetes.Interface,
	namespace, name string,
) (int, bool) {
	slices, err := client.DiscoveryV1().EndpointSlices(namespace).List(
		ctx, metav1.ListOptions{
			LabelSelector: discoveryv1.LabelServiceName + "=" + name,
		},
	)
	if err != nil {
		return 0, false
	}
	ready := 0
	for _, slice := range slices.Items {
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Terminating != nil &&
				*endpoint.Conditions.Terminating {
				continue
			}
			if endpoint.Conditions.Serving != nil &&
				!*endpoint.Conditions.Serving {
				continue
			}
			if endpoint.Conditions.Ready == nil ||
				*endpoint.Conditions.Ready {
				ready++
			}
		}
	}
	return ready, true
}
