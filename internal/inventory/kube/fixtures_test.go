package kube_test

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory"
)

const testNamespace = "testns"

func pod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "app:1.0"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type: corev1.PodReady, Status: corev1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(fixedTime()),
				},
			},
		},
	}
}

func node(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubelet", Time: timePtr(fixedTime())},
			},
		},
		Spec: corev1.NodeSpec{},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type: corev1.NodeReady, Status: corev1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(fixedTime()),
				},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:          "v1.25.0",
				ContainerRuntimeVersion: "docker://20.10.0",
				KernelVersion:           "5.10.0",
				OperatingSystem:         "linux",
			},
		},
	}
}

func secret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"key1": []byte("value1"),
		},
	}
}

func configMap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(name),
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
		},
		Data: map[string]string{
			"config.yaml": "data",
		},
	}
}

func fixedTime() time.Time {
	return time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
}

func timePtr(t time.Time) *metav1.Time {
	mt := metav1.NewTime(t)
	return &mt
}

func boolPtr(b bool) *bool {
	return &b
}

func mustQuantity(s string) resource.Quantity {
	q, _ := resource.ParseQuantity(s)
	return q
}

// observationsByKind groups observations by kind for easier assertion.
func observationsByKind(
	observations []inventory.Observation,
) map[inventory.ObservationKind][]inventory.Observation {
	out := make(map[inventory.ObservationKind][]inventory.Observation)
	for _, f := range observations {
		out[f.Kind] = append(out[f.Kind], f)
	}
	return out
}
