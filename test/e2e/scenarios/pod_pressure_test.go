//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodEphemeralStorageEviction(t *testing.T) {
	runScenario(t, "pod.ephemeral-storage-pressure", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		_, err := e.Client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "disk-pressure"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "disk"},
				ImagePullPolicy: corev1.PullNever,
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceEphemeralStorage: resource.MustParse("1Mi"),
					},
				},
			}}},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.WaitForPod(waitCtx, namespace, "disk-pressure",
			func(pod *corev1.Pod) bool {
				return pod.Status.Reason == "Evicted" ||
					pod.Status.Phase == corev1.PodFailed &&
						pod.Status.Reason == "Evicted"
			},
		); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(waitCtx, harness.AuditMatch{
			Namespace: namespace, Resource: "disk-pressure",
			Reason: "Evicted", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}
