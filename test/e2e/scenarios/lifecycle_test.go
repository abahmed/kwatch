//go:build e2e

package scenarios

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioResolution(t *testing.T) {
	inNamespace(t, "lifecycle.resolution", func(s *Scenario) {
		_, err := createLifecycleDeployment(
			s.Ctx, s.Env, s.Namespace, "recovery", "crash")
		s.Must(err)
		s.ExpectIncident("recovery", "CrashLoopBackOff", 0)

		s.Must(setLifecycleMode(
			s.Ctx, s.Env, s.Namespace, "recovery", "healthy"))
		s.ExpectResolved("recovery", "CrashLoopBackOff")
	})
}

func TestScenarioRefailureAfterRecovery(t *testing.T) {
	inNamespace(t, "pod.re-failure-after-recovery", func(s *Scenario) {
		_, err := createLifecycleDeployment(
			s.Ctx, s.Env, s.Namespace, "refailure", "crash")
		s.Must(err)
		s.ExpectIncident("refailure", "CrashLoopBackOff", 0)

		s.Must(setLifecycleMode(
			s.Ctx, s.Env, s.Namespace, "refailure", "healthy"))
		s.ExpectResolved("refailure", "CrashLoopBackOff")

		s.Must(setLifecycleMode(
			s.Ctx, s.Env, s.Namespace, "refailure", "crash"))
		lifecycleExpectIncidents(s, "refailure", "CrashLoopBackOff", 2)
	})
}

func TestScenarioRestartPersistence(t *testing.T) {
	inNamespace(t, "lifecycle.restart-persistence", func(s *Scenario) {
		s.Must(s.Env.Receiver.Clear(s.Ctx))
		_, err := createLifecycleDeployment(
			s.Ctx, s.Env, s.Namespace, "persistent", "crash")
		s.Must(err)
		s.ExpectIncident("persistent", "CrashLoopBackOff", 0)
		sent := harness.DeliveryMatch{
			Name: "persistent", Reason: "CrashLoopBackOff",
		}
		_, err = s.Env.Receiver.WaitForMatchCount(s.Ctx, sent, 1)
		s.Must(err)

		lifecycleDeleteLeader(s)
		deliveries, err := s.Env.Receiver.Matching(s.Ctx, sent)
		s.Must(err)
		if len(deliveries) != 1 {
			t.Fatalf("restart changed delivery count: %d", len(deliveries))
		}
		s.Must(s.Env.AssertHealthy(s.Ctx))
	})
}

// TestScenarioLeaseHandover deletes the only Pod. The replacement must
// acquire the Lease, become available, and keep detecting new incidents.
func TestScenarioLeaseHandover(t *testing.T) {
	inNamespace(t, "lifecycle.lease-handover", func(s *Scenario) {
		oldHolder, newHolder := lifecycleDeleteLeader(s)
		if newHolder == oldHolder {
			t.Fatalf("Lease holder did not change from %q", oldHolder)
		}
		s.Must(s.Env.Health.AssertOK(s.Ctx, "/availabilityz"))

		_, err := createLifecycleDeployment(
			s.Ctx, s.Env, s.Namespace, "takeover", "crash")
		s.Must(err)
		s.ExpectIncident("takeover", "CrashLoopBackOff", 0)
	})
}

func createLifecycleDeployment(
	ctx context.Context,
	e *harness.Environment,
	namespace, name, command string,
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
						Name:  "workload",
						Image: workloadImage(),
						Command: []string{
							"/kwatch-e2e-workload", command,
						},
						ImagePullPolicy: corev1.PullNever,
					}}},
				},
			},
		}, metav1.CreateOptions{})
}

// setLifecycleMode switches the workload of a lifecycle Deployment. It
// re-reads the Deployment on every attempt, because the Deployment
// controller updates it concurrently.
func setLifecycleMode(
	ctx context.Context,
	e *harness.Environment,
	namespace, name, mode string,
) error {
	deployments := e.Client.AppsV1().Deployments(namespace)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		deployment.Spec.Template.Spec.Containers[0].Command = []string{
			"/kwatch-e2e-workload", mode,
		}
		_, err = deployments.Update(ctx, deployment, metav1.UpdateOptions{})
		return err
	})
}
