//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodLifecycleHookFailure(t *testing.T) {
	inNamespace(t, "pod.lifecycle-hook", func(s *Scenario) {
		s.CreatePod(podWithFailingStartHook("post-start"))
		s.ExpectIncident("post-start", "FailedPostStartHook", 0)
	})
}

// A suspended CronJob is advisory: it is context inside a real failure's
// incident and never an incident of its own, so Kwatch stays quiet.
func TestScenarioCronJobSuspended(t *testing.T) {
	inNamespace(t, "workload.cronjob", func(s *Scenario) {
		s.CreateCronJob(suspendedCronJob("suspended"))
		s.ExpectNoIncidentFor("suspended", announceWait(0))
	})
}

// ExpectNoIncidentFor fails if Kwatch announces anything about resource
// within window. Running out the window is the pass.
func (s *Scenario) ExpectNoIncidentFor(resource string, window time.Duration) {
	s.T.Helper()
	err := wait.PollUntilContextTimeout(s.Ctx, 5*time.Second, window, false,
		func(ctx context.Context) (bool, error) {
			entries, err := s.Env.Audit.Matching(ctx, harness.AuditMatch{
				Namespace: s.Namespace, Resource: resource,
			})
			if err != nil {
				return false, err
			}
			if len(entries) > 0 {
				s.T.Fatalf("unexpected incident for %s: %+v",
					resource, entries[0])
			}
			return false, nil
		})
	if err != nil && !wait.Interrupted(err) {
		s.Must(err)
	}
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
		s.ExpectGatheredEvidence(s.Namespace, "missing-secret")
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
