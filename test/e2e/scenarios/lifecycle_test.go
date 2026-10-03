//go:build e2e

package scenarios

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// The scenarios below break a Deployment with a missing ConfigMap and fix
// it by creating the ConfigMap. A crash loop would not do: its pod flips
// between failing and recovering, and every flip doubles the time Kwatch
// holds the incident open before resolving it.

func TestScenarioResolution(t *testing.T) {
	inNamespace(t, "lifecycle.resolution", func(s *Scenario) {
		lifecycleCreateNeedyDeployment(s, "recovery", "settings")
		s.ExpectIncident("settings", "ProjectedConfigMapMissing", 0)

		// Once the ConfigMap exists the incident is rooted at the
		// Deployment, so that is the root that resolves.
		lifecycleFixMissingConfigMap(s, "recovery", "settings")
		s.ExpectResolved("recovery")
	})
}

func TestScenarioRefailureAfterRecovery(t *testing.T) {
	inNamespace(t, "pod.re-failure-after-recovery", func(s *Scenario) {
		lifecycleCreateNeedyDeployment(s, "refailure", "settings")
		s.ExpectIncident("settings", "ProjectedConfigMapMissing", 0)

		lifecycleFixMissingConfigMap(s, "refailure", "settings")
		s.ExpectResolved("refailure")

		// Pods that already started keep their environment, so the
		// problem only returns when the Pods are replaced.
		lifecycleDeleteConfigMap(s, "settings")
		lifecycleRestartPods(s, "refailure")
		lifecycleExpectIncidents(s, "settings", "ProjectedConfigMapMissing", 2)
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
		s.ExpectKwatchHealthy()
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
