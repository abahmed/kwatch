package controlplane

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponentCandidatesAndReasons(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			"component": "etcd",
		}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			"k8s-app": "etcd",
		}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			"component": "kube-scheduler",
		}}},
	}
	if got := componentCandidates("etcd", pods); len(got) != 2 {
		t.Fatalf("etcd candidates = %d, want 2", len(got))
	}
	if got := componentCandidates("missing", pods); len(got) != 0 {
		t.Fatalf("missing candidates = %d, want 0", len(got))
	}
	tests := map[string]string{
		"kube-scheduler":          "SchedulerUnavailable",
		"kube-controller-manager": "ControllerManagerUnavailable",
		"etcd":                    "EtcdUnavailable",
		"other":                   "ControlPlaneComponentFailure",
	}
	for component, want := range tests {
		if got := componentReason(component); got != want {
			t.Fatalf("componentReason(%q) = %q, want %q",
				component, got, want)
		}
	}
}
