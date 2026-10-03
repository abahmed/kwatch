//go:build e2e

package scenarios

import (
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodLifecycleHookFailure(t *testing.T) {
	inNamespace(t, "pod.lifecycle-hook", func(s *Scenario) {
		s.Must(edgeCreatePod(
			s.Ctx, s.Env, s.Namespace, edgeFailingHookPod("post-start")))
		s.ExpectIncident("post-start", "FailedPostStartHook", 0)
	})
}

func TestScenarioCronJobSuspended(t *testing.T) {
	inNamespace(t, "workload.cronjob", func(s *Scenario) {
		s.Must(edgeCreateSuspendedCronJob(
			s.Ctx, s.Env, s.Namespace, "suspended"))
		s.ExpectIncident("suspended", "CronJobSuspended", 0)
	})
}

func TestScenarioMissingRequiredReferences(t *testing.T) {
	inNamespace(t, "security.secret-reference", func(s *Scenario) {
		pod := edgePodWithEnvFrom(
			"missing-references", edgeSecretEnvFrom("missing-secret"))
		s.Must(edgeCreatePod(s.Ctx, s.Env, s.Namespace, pod))
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
		pod := edgePodWithEnvFrom(
			"missing-configmap", edgeConfigMapEnvFrom("missing-configmap"))
		s.Must(edgeCreatePod(s.Ctx, s.Env, s.Namespace, pod))
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
		s.Must(edgeCreatePodWithDeletedServiceAccount(
			s.Ctx, s.Env, s.Namespace, "missing-service-account"))
		s.ExpectIncident(
			"missing-service-account", "ServiceAccountMissing", 0)
	})
}

func TestScenarioMissingIngressBackend(t *testing.T) {
	inNamespace(t, "networking.ingress", func(s *Scenario) {
		s.Must(edgeCreateIngressToMissingService(
			s.Ctx, s.Env, s.Namespace, "missing-backend", "missing-service"))
		s.ExpectIncident("missing-service", "IngressBackendNotFound", 0)
	})
}

func TestScenarioRestrictiveNetworkPolicy(t *testing.T) {
	inNamespace(t, "networking.network-policy", func(s *Scenario) {
		s.Must(edgeCreateDenyAllEgressPolicy(
			s.Ctx, s.Env, s.Namespace, "deny-egress"))
		s.ExpectIncident("deny-egress", "RestrictiveNetworkPolicy", 0)
	})
}

func TestScenarioPodSecurityAdmission(t *testing.T) {
	inNamespace(t, "security.pod-security-admission", func(s *Scenario) {
		s.Must(edgeEnforceRestrictedSecurity(s.Ctx, s.Env, s.Namespace))
		err := edgeCreatePod(
			s.Ctx, s.Env, s.Namespace, edgePrivilegedPod("privileged"))
		if !apierrors.IsForbidden(err) {
			t.Fatalf("expected Pod Security Admission rejection, got %v", err)
		}
		s.ExpectKwatchHealthy()
	})
}
