package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestPortMatchAttributes(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
			{Port: 80, Name: "http", TargetPort: intstr.FromString("web")},
			{Port: 9000},
		}},
	}
	got, _ := kube.ServiceSchema{}.Describe(svc)
	assert.Equal(t, "80:http:web,9000::9000",
		got.Attributes[kube.AttrServicePortSpecs].AsText())

	prefix := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "shop"},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path: "/api", PathType: &prefix,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: "api",
								Port: networkingv1.ServiceBackendPort{
									Number: 9090}}}}}}},
		}}},
	}
	described, _ := kube.IngressSchema{}.Describe(ing)
	assert.Equal(t, "/api\tapi\t9090",
		described.Attributes[kube.AttrIngressBackendPorts].AsText())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-1"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "app", Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: 80}, {ContainerPort: 9100}},
		}}},
	}
	podDesc, _ := kube.PodSchema{}.Describe(pod)
	assert.Equal(t, "http=80,9100",
		podDesc.Attributes[kube.AttrPodPorts].AsText())
}
