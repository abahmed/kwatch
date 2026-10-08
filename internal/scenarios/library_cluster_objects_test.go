package scenarios

import (
	"fmt"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// Object builders shared by the cluster scenarios.

func clusterHPA(
	c *cluster, namespace, name, active, reason, message string,
) *autoscalingv2.HorizontalPodAutoscaler {
	meta := clusterMeta(c, namespace, name, "hpa")
	minReplicas := int32(2)
	since := metav1.NewTime(c.now)
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: meta,
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1", Kind: "Deployment", Name: meta.Name,
			},
			MinReplicas: &minReplicas, MaxReplicas: 10,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 2, DesiredReplicas: 2,
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{Type: autoscalingv2.AbleToScale, Status: "True",
					Reason: "ReadyForNewScale", LastTransitionTime: since},
				{Type: autoscalingv2.ScalingActive,
					Status: corev1.ConditionStatus(active), Reason: reason,
					Message: message, LastTransitionTime: since},
			},
		},
	}
}

func clusterAPIService(
	c *cluster, status, reason, message string,
) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{
			"name": "v1beta1.metrics.k8s.io", "uid": "apiservice-metrics",
		},
		"spec": map[string]any{
			"group": "metrics.k8s.io", "version": "v1beta1",
			"service": map[string]any{
				"namespace": c.n("kube-system"),
				"name":      c.n("metrics-server"),
			},
		},
		"status": map[string]any{"conditions": []any{map[string]any{
			"type": "Available", "status": status, "reason": reason,
			"message":            message,
			"lastTransitionTime": c.now.UTC().Format(time.RFC3339),
		}}},
	}}
	return u
}

func clusterService(
	c *cluster, namespace, name string, port int32,
) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: clusterMeta(c, namespace, name, "svc"),
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": c.n(name)},
			Ports:    []corev1.ServicePort{{Port: port}},
		},
	}
}

// clusterSlice is the Service's EndpointSlice with one ready endpoint
// per pod; no pods means no endpoints.
func clusterSlice(
	c *cluster, namespace, service string, pods ...*corev1.Pod,
) *discoveryv1.EndpointSlice {
	meta := clusterMeta(c, namespace, service+"-x1", "slice")
	meta.Labels = map[string]string{discoveryv1.LabelServiceName: c.n(service)}
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: meta, AddressType: discoveryv1.AddressTypeIPv4,
	}
	for i, pod := range pods {
		slice.Endpoints = append(slice.Endpoints, discoveryv1.Endpoint{
			Addresses:  []string{fmt.Sprintf("10.244.0.%d", 10+i)},
			Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)},
			TargetRef: &corev1.ObjectReference{
				Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name,
			},
		})
	}
	return slice
}

func clusterValidatingHook(
	c *cluster, name, webhook, namespace, service string,
) *admissionv1.ValidatingWebhookConfiguration {
	fail := admissionv1.Fail
	none := admissionv1.SideEffectClassNone
	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: clusterMeta(c, "", name, "hook"),
		Webhooks: []admissionv1.ValidatingWebhook{{
			Name: webhook, FailurePolicy: &fail, SideEffects: &none,
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Namespace: c.n(namespace), Name: c.n(service),
				},
			},
		}},
	}
}

// clusterMeta is c.meta with a UID distinct per kind, so a Service and
// the Deployment of the same name never share one.
func clusterMeta(c *cluster, namespace, name, kind string) metav1.ObjectMeta {
	meta := c.meta(namespace, name)
	meta.UID += "-" + types.UID(kind)
	return meta
}
