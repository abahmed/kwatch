//go:build e2e

package scenarios

import (
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodLifecycleHookFailure(t *testing.T) {
	inNamespace(t, "pod.lifecycle-hook", func(s *Scenario) {
		s.CreatePod(podWithFailingStartHook("post-start"))
		s.ExpectIncident("post-start", "FailedPostStartHook", 0)
	})
}

func TestScenarioCronJobSuspended(t *testing.T) {
	inNamespace(t, "workload.cronjob", func(s *Scenario) {
		s.CreateCronJob(suspendedCronJob("suspended"))
		s.ExpectIncident("suspended", "CronJobSuspended", 0)
	})
}

func TestScenarioMissingRequiredReferences(t *testing.T) {
	inNamespace(t, "security.secret-reference", func(s *Scenario) {
		s.CreatePod(podWithEnvFrom(
			"missing-references", secretEnvFrom("missing-secret")))
		blamed := "pod/" + s.Namespace + "/missing-references"
		s.ExpectRoot(harness.RootExpectation{
			Root:         "secret/" + s.Namespace + "/missing-secret",
			Tier:         "notify",
			MaxMessages:  2,
			MustNotBlame: []string{blamed},
		})
	})
}

func TestScenarioMissingConfigMapReference(t *testing.T) {
	inNamespace(t, "security.configmap-reference", func(s *Scenario) {
		s.CreatePod(podWithEnvFrom(
			"missing-configmap", configMapEnvFrom("missing-configmap")))
		s.ExpectRoot(harness.RootExpectation{
			Root:         "configmap/" + s.Namespace + "/missing-configmap",
			Tier:         "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"pod/" + s.Namespace + "/missing-configmap"},
		})
	})
}

func TestScenarioMissingServiceAccountReference(t *testing.T) {
	inNamespace(t, "security.rbac", func(s *Scenario) {
		s.CreatePodWithDeletedServiceAccount("missing-service-account")
		// Kwatch sees the missing ServiceAccount only after the Pod's
		// mount failures start, which takes a couple of minutes.
		s.ExpectIncident("missing-service-account",
			"ServiceAccountMissing", 2*time.Minute)
	})
}

func TestScenarioMissingIngressBackend(t *testing.T) {
	inNamespace(t, "networking.ingress", func(s *Scenario) {
		s.CreateIngress(ingressToMissingService(
			"missing-backend", "missing-service"))
		// A new Ingress may wait for its backend Service for a grace period.
		s.ExpectIncident("missing-service", "IngressBackendNotFound",
			detectors.DefaultBackendGrace)
	})
}

func TestScenarioRestrictiveNetworkPolicy(t *testing.T) {
	inNamespace(t, "networking.network-policy", func(s *Scenario) {
		s.CreateNetworkPolicy(denyAllEgressPolicy("deny-egress"))
		s.ExpectIncident("deny-egress", "RestrictiveNetworkPolicy", 0)
	})
}

func TestScenarioPodSecurityAdmission(t *testing.T) {
	inNamespace(t, "security.pod-security-admission", func(s *Scenario) {
		s.EnforceRestrictedPodSecurity()
		err := s.TryCreatePod(privilegedPod("privileged"))
		if !apierrors.IsForbidden(err) {
			t.Fatalf("expected Pod Security Admission rejection, got %v", err)
		}
		s.ExpectKwatchHealthy()
	})
}
