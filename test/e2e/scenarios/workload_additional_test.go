//go:build e2e

package scenarios

import (
	"testing"
	"time"
)

// Both findings need five minutes of unavailability, which is longer than
// the incident waits, so the incident is announced for the pod's own
// failure, whose image is not on the node.
func TestScenarioStatefulSetFailure(t *testing.T) {
	inNamespace(t, "workload.statefulset", func(s *Scenario) {
		s.CreateStatefulSet(stuckStatefulSet("database"))
		s.ExpectIncident("database", "ErrImageNeverPull", 0)
	})
}

func TestScenarioDaemonSetFailure(t *testing.T) {
	inNamespace(t, "workload.daemonset", func(s *Scenario) {
		s.CreateDaemonSet(stuckDaemonSet("agent"))
		s.ExpectIncident("agent", "ErrImageNeverPull", 0)
	})
}

// A budget that cannot be met is raised only after ten minutes, counted
// from when Kwatch first sees it, so the scenario starts early.
func TestScenarioPDBDisruption(t *testing.T) {
	inNamespaceEarly(t, "workload.pdb-disruption", func(s *Scenario) {
		s.CreateDeployment("protected", "crash")
		s.CreatePodDisruptionBudget(
			podDisruptionBudget("protected", "protected"))
		s.ExpectIncident("protected", "PdbViolation", 10*time.Minute)
	})
}

// The quota, not the ReplicaSet, is the root of the incident.
func TestScenarioReplicaSetFailure(t *testing.T) {
	inNamespace(t, "workload.replicaset", func(s *Scenario) {
		s.CreateResourceQuota(zeroPodQuota("zero-pods"))
		s.CreateReplicaSet(healthyReplicaSet("blocked"))
		s.ExpectIncident("zero-pods", "ReplicaSetFailure", 0)
	})
}
