//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioDeploymentRolloutFailure(t *testing.T) {
	runScenario(t, "workload.deployment-rollout", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		deployment, err := createHealthyDeployment(ctx, e, namespace, "rollout")
		if err != nil {
			t.Fatal(err)
		}
		if err := e.WaitForDeployment(ctx, namespace, deployment.Name); err != nil {
			t.Fatal(err)
		}
		deployment.Spec.Template.Spec.Containers[0].Image =
			"example.invalid/kwatch/missing:rollout"
		deployment.Spec.ProgressDeadlineSeconds = int32Ptr(5)
		if _, err := e.Client.AppsV1().Deployments(namespace).Update(
			ctx, deployment, metav1.UpdateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := waitForDeploymentReason(waitCtx, e, namespace,
			deployment.Name, "ProgressDeadlineExceeded"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: deployment.Name,
			Reason: "ProgressDeadlineExceeded", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioJobFailure(t *testing.T) {
	runScenario(t, "workload.job", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		backoff := int32(0)
		_, err := e.Client.BatchV1().Jobs(namespace).Create(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "failed-job"},
			Spec: batchv1.JobSpec{
				BackoffLimit: &backoff,
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name: "workload", Image: workloadImage(),
						Command:         []string{"/kwatch-e2e-workload", "crash"},
						ImagePullPolicy: corev1.PullNever,
					}},
				}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := waitForJobFailure(waitCtx, e, namespace, "failed-job"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
			Namespace: namespace, Resource: "failed-job",
			Reason: "JobFailed", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func createHealthyDeployment(
	ctx context.Context,
	e *harness.Environment,
	namespace, name string,
) (*appsv1.Deployment, error) {
	return e.Client.AppsV1().Deployments(namespace).Create(ctx,
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
						Name: "workload", Image: workloadImage(),
						Command:         []string{"/kwatch-e2e-workload", "healthy"},
						ImagePullPolicy: corev1.PullNever,
					}},
					},
				},
			}}, metav1.CreateOptions{})
}

func waitForDeploymentReason(
	ctx context.Context,
	e *harness.Environment,
	namespace, name, reason string,
) error {
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond,
		5*time.Minute, true, func(ctx context.Context) (bool, error) {
			deployment, err := e.Client.AppsV1().Deployments(namespace).Get(
				ctx, name, metav1.GetOptions{},
			)
			if err != nil {
				return false, nil
			}
			for _, condition := range deployment.Status.Conditions {
				if condition.Reason == reason {
					return true, nil
				}
			}
			return false, nil
		})
}

func waitForJobFailure(
	ctx context.Context,
	e *harness.Environment,
	namespace, name string,
) error {
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond,
		5*time.Minute, true, func(ctx context.Context) (bool, error) {
			job, err := e.Client.BatchV1().Jobs(namespace).Get(
				ctx, name, metav1.GetOptions{},
			)
			return err == nil && job.Status.Failed > 0, nil
		})
}
