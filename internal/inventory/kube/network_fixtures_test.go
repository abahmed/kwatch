package kube_test

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func service(name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": "test",
			},
			Ports: []corev1.ServicePort{
				{Port: 8080, Protocol: corev1.ProtocolTCP},
			},
		},
	}
}

func endpointSlice(name string) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			Labels: map[string]string{
				discoveryv1.LabelServiceName: "mysvc",
			},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "controller", Time: timePtr(fixedTime())},
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.0.0.1"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: boolPtr(true),
				},
				TargetRef: &corev1.ObjectReference{
					Kind: "Pod", Name: "pod1", Namespace: testNamespace,
				},
			},
		},
	}
}

func ingress(name string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "nginx", Time: timePtr(fixedTime())},
			},
		},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "mysvc",
					Port: networkingv1.ServiceBackendPort{Number: 8080},
				},
			},
		},
	}
}
