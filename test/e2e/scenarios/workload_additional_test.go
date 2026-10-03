//go:build e2e

package scenarios

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Both findings need five minutes of unavailability, which is longer than
// the incident waits, so the incident is announced for the pod's own
// failure, whose image is not on the node.
func TestScenarioStatefulSetFailure(t *testing.T) {
	inNamespace(t, "workload.statefulset", func(s *Scenario) {
		_, err := s.Env.Client.AppsV1().StatefulSets(s.Namespace).Create(
			s.Ctx, workloadsStatefulSet("database"), metav1.CreateOptions{})
		s.Must(err)
		s.ExpectIncident("database", "ErrImageNeverPull", 0)
	})
}

func TestScenarioDaemonSetFailure(t *testing.T) {
	inNamespace(t, "workload.daemonset", func(s *Scenario) {
		_, err := s.Env.Client.AppsV1().DaemonSets(s.Namespace).Create(
			s.Ctx, workloadsDaemonSet("agent"), metav1.CreateOptions{})
		s.Must(err)
		s.ExpectIncident("agent", "ErrImageNeverPull", 0)
	})
}

// A budget that cannot be met is raised only after ten minutes.
func TestScenarioPDBDisruption(t *testing.T) {
	inNamespace(t, "workload.pdb-disruption", func(s *Scenario) {
		_, err := createLifecycleDeployment(
			s.Ctx, s.Env, s.Namespace, "protected", "crash")
		s.Must(err)
		_, err = s.Env.Client.PolicyV1().PodDisruptionBudgets(s.Namespace).
			Create(s.Ctx, workloadsPDB("protected-budget", "protected"),
				metav1.CreateOptions{})
		s.Must(err)
		s.ExpectIncident("protected-budget", "PdbViolation",
			10*time.Minute)
	})
}

// The quota, not the ReplicaSet, is the root of the incident.
func TestScenarioReplicaSetFailure(t *testing.T) {
	inNamespace(t, "workload.replicaset", func(s *Scenario) {
		_, err := s.Env.Client.CoreV1().ResourceQuotas(s.Namespace).Create(
			s.Ctx, workloadsZeroPodQuota("zero-pods"),
			metav1.CreateOptions{})
		s.Must(err)
		_, err = s.Env.Client.AppsV1().ReplicaSets(s.Namespace).Create(
			s.Ctx, workloadsReplicaSet("blocked"), metav1.CreateOptions{})
		s.Must(err)
		s.ExpectIncident("zero-pods", "ReplicaSetFailure", 0)
	})
}
