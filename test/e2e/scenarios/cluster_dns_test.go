//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// TestScenarioExtendedClusterDNSDown scales CoreDNS to zero and expects a
// cluster-dns page rather than blame on nodes or consumers. CoreDNS is
// restored before the scenario ends.
func TestScenarioExtendedClusterDNSDown(t *testing.T) {
	inExtendedCluster(t, "integration.cluster-dns", func(s *Scenario) {
		extScaleCoreDNSToZero(s)
		s.ExpectRoot(harness.RootExpectation{
			Root:         "cluster-dns//cluster-dns",
			Tier:         "page",
			MaxMessages:  2,
			MustNotBlame: extKindNodeRoots(s),
		})
	})
}

func extKindNodeRoots(s *Scenario) []string {
	s.T.Helper()
	nodes, err := s.Env.Client.CoreV1().Nodes().List(s.Ctx,
		metav1.ListOptions{})
	s.Must(err)
	roots := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		roots = append(roots, "node//"+node.Name)
	}
	return roots
}

// extScaleCoreDNSToZero stops CoreDNS and scales it back to its original
// size when the test ends.
func extScaleCoreDNSToZero(s *Scenario) {
	s.T.Helper()
	deployments := s.Env.Client.AppsV1().Deployments("kube-system")
	scale, err := deployments.GetScale(s.Ctx, "coredns", metav1.GetOptions{})
	s.Must(err)
	original := scale.Spec.Replicas
	scale.Spec.Replicas = 0
	_, err = deployments.UpdateScale(
		s.Ctx, "coredns", scale, metav1.UpdateOptions{})
	s.Must(err)
	s.T.Cleanup(func() { extRestoreCoreDNS(s, original) })
}

func extRestoreCoreDNS(s *Scenario, replicas int32) {
	t := s.T
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	deployments := s.Env.Client.AppsV1().Deployments("kube-system")
	scale, err := deployments.GetScale(ctx, "coredns", metav1.GetOptions{})
	if err != nil {
		t.Errorf("read CoreDNS scale: %v", err)
		return
	}
	scale.Spec.Replicas = replicas
	if _, err := deployments.UpdateScale(
		ctx, "coredns", scale, metav1.UpdateOptions{},
	); err != nil {
		t.Errorf("restore CoreDNS: %v", err)
		return
	}
	err = wait.PollUntilContextTimeout(ctx, time.Second, 3*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			current, getErr := deployments.Get(
				ctx, "coredns", metav1.GetOptions{})
			return getErr == nil &&
				current.Status.ReadyReplicas >= replicas, nil
		})
	if err != nil {
		t.Errorf("wait for CoreDNS: %v", err)
	}
}
