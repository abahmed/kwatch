//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioGroupingDifferentReasons(t *testing.T) {
	runScenario(t, "grouping.different-reasons", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		if err := e.Receiver.Clear(ctx); err != nil {
			t.Fatal(err)
		}
		if err := createReasonDeployment(ctx, e, namespace, "crash",
			workloadImage(), []string{"/kwatch-e2e-workload", "crash"}); err != nil {
			t.Fatal(err)
		}
		if err := createReasonDeployment(ctx, e, namespace, "pull",
			"example.invalid/kwatch/missing:never", []string{"sleep"}); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		for name, reason := range map[string]string{
			"crash": "CrashLoopBackOff", "pull": "ImagePullBackOff",
		} {
			if err := e.WaitForPod(waitCtx, namespace, name,
				func(pod *corev1.Pod) bool {
					return harness.PodHasReason(pod, reason)
				},
			); err != nil {
				t.Fatal(err)
			}
			if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
				Namespace: namespace, Resource: name, Reason: reason,
				Action: "create", Count: 1,
			}); err != nil {
				t.Fatal(err)
			}
		}
		requests, err := e.Receiver.WaitForCount(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) < 2 {
			t.Fatalf("different reasons were grouped: %d deliveries", len(requests))
		}
	})
}

func TestScenarioGroupingCrossNamespace(t *testing.T) {
	runScenario(t, "grouping.cross-namespace", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespaces := []string{uniqueNamespace(t.Name() + "-one"),
			uniqueNamespace(t.Name() + "-two")}
		for _, namespace := range namespaces {
			if err := createNamespace(ctx, e, namespace); err != nil {
				t.Fatal(err)
			}
			defer cleanupNamespace(t, e, namespace)
			if err := createFailingDeployment(ctx, e, namespace, "worker"); err != nil {
				t.Fatal(err)
			}
		}
		requests, err := e.Receiver.WaitForCount(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) < 2 {
			t.Fatalf(
				"cross-namespace failures were grouped: %d deliveries",
				len(requests),
			)
		}
	})
}

func createReasonDeployment(
	ctx context.Context,
	e *harness.Environment,
	namespace, name, image string,
	command []string,
) error {
	_, err := e.Client.AppsV1().Deployments(namespace).Create(ctx,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"app": name,
				}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
						"app": name,
					}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: "workload", Image: image, Command: command,
						ImagePullPolicy: corev1.PullNever,
					}}},
				},
			},
		}, metav1.CreateOptions{})
	return err
}
