//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// The scenarios below break a Deployment with a missing ConfigMap and fix
// it by creating the ConfigMap. A crash loop would not do: its pod flips
// between failing and recovering, and every flip doubles the time Kwatch
// holds the incident open before resolving it.

func TestScenarioResolution(t *testing.T) {
	inNamespace(t, "lifecycle.resolution", func(s *Scenario) {
		s.CreateDeployment("recovery", "healthy",
			withConfigMapEnv("settings"))
		incident := s.ExpectIncident(
			"settings", "ProjectedConfigMapMissing", 0)

		s.FixMissingConfigMap("recovery", "settings")
		s.ExpectResolved(incident)
	})
}

func TestScenarioRefailureAfterRecovery(t *testing.T) {
	inNamespace(t, "pod.re-failure-after-recovery", func(s *Scenario) {
		s.CreateDeployment("refailure", "healthy",
			withConfigMapEnv("settings"))
		incident := s.ExpectIncident(
			"settings", "ProjectedConfigMapMissing", 0)

		s.FixMissingConfigMap("refailure", "settings")
		s.ExpectResolved(incident)

		// Pods that already started keep their environment, so the
		// problem only returns when the Pods are replaced.
		s.DeleteConfigMap("settings")
		s.RestartPods("refailure")
		s.ExpectIncidents("settings", "ProjectedConfigMapMissing", 2)
	})
}

func TestScenarioRestartPersistence(t *testing.T) {
	inNamespaceAlone(t, "lifecycle.restart-persistence", func(s *Scenario) {
		s.ClearReceiver()
		s.CreateDeployment("persistent", "crash")
		s.ExpectIncident("persistent", "CrashLoopBackOff", 0)
		sent := harness.DeliveryMatch{
			Name: "persistent", Reason: "CrashLoopBackOff",
		}
		s.WaitForDeliveries(sent, 1)

		s.DeleteLeader()
		s.ExpectDeliveries(sent, 1)
		s.ExpectKwatchHealthy()
	})
}

// TestScenarioLeaseHandover deletes the only Pod. The replacement must
// acquire the Lease, become available, and keep detecting new incidents.
func TestScenarioLeaseHandover(t *testing.T) {
	inNamespaceAlone(t, "lifecycle.lease-handover", func(s *Scenario) {
		oldHolder, newHolder := s.DeleteLeader()
		if newHolder == oldHolder {
			t.Fatalf("Lease holder did not change from %q", oldHolder)
		}
		s.ExpectKwatchAvailable()

		s.CreateDeployment("takeover", "crash")
		s.ExpectIncident("takeover", "CrashLoopBackOff", 0)
	})
}
