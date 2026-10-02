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
	runExtendedScenario(t, "integration.cluster-dns", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		deployments := e.Client.AppsV1().Deployments("kube-system")
		scale, err := deployments.GetScale(ctx, "coredns", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		original := scale.Spec.Replicas
		started := time.Now()
		scale.Spec.Replicas = 0
		if _, err := deployments.UpdateScale(
			ctx, "coredns", scale, metav1.UpdateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		defer restoreCoreDNS(t, e, original)
		assertRoot(ctx, t, e, "", started, harness.RootExpectation{
			Root:         "cluster-dns//cluster-dns",
			Tier:         "page",
			MaxMessages:  2,
			MustNotBlame: kindNodeRoots(ctx, t, e),
		})
	})
}

func kindNodeRoots(
	ctx context.Context, t *testing.T, e *harness.Environment,
) []string {
	t.Helper()
	nodes, err := e.Client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	roots := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		roots = append(roots, "node//"+node.Name)
	}
	return roots
}

func restoreCoreDNS(t *testing.T, e *harness.Environment, replicas int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	deployments := e.Client.AppsV1().Deployments("kube-system")
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
				ctx, "coredns", metav1.GetOptions{},
			)
			return getErr == nil &&
				current.Status.ReadyReplicas >= replicas, nil
		})
	if err != nil {
		t.Errorf("wait for CoreDNS: %v", err)
	}
}
