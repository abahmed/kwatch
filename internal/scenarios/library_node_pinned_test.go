package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nodePinnedScenarios are nodes lost under workloads that have nowhere
// else to run.
func nodePinnedScenarios() []scenario {
	return []scenario{nodeLostPinnedTenants()}
}

// pendingOnNode is a replacement pod bound to the lost node: the
// scheduler is skipped, so it shows no condition, only Phase Pending.
func pendingOnNode(c *cluster, pod *corev1.Pod) {
	now := metav1.NewTime(c.now)
	pod.CreationTimestamp = now
	pod.Status = corev1.PodStatus{
		Phase: corev1.PodPending, QOSClass: corev1.PodQOSBestEffort,
	}
}

// nodeLostPinnedTenants: six one-replica workloads are pinned to n1 and
// n1 stops. The pods go not ready when the node turns Unknown; five
// minutes later the taint manager deletes them and each replacement
// stays Pending on the dead node. No replica runs anywhere else, yet the
// node is the root, not six separate deployments.
func nodeLostPinnedTenants() scenario {
	names := make([]string, 0, 6)
	for i := range 6 {
		names = append(names, fmt.Sprintf("tenant-%d", i))
	}
	return scenario{
		expect: expectation{
			Name: "node-lost-pinned-tenants",
			Description: "A node stops reporting; six one-replica " +
				"workloads pinned to it go not ready, are deleted " +
				"after five minutes and their replacements stay " +
				"Pending on the dead node.",
			Root: "node//n1", Tier: "page", MaxMessages: 3,
			MustNotBlame: []string{"deployment/apps/tenant-0",
				"deployment/apps/tenant-3", "deployment/apps/tenant-5"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			var fleet []*workload
			for _, name := range names {
				w := c.deployment("apps", name,
					"registry.example.com/"+name+":1.0", 1)
				c.list(w.objects())
				c.list(w.pod(0, "n1"))
				fleet = append(fleet, w)
			}
			c.after(time.Minute)
			nodeLose(c, n1)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(0)
				c.update(w.deployment, w.replicaSet)
			}
			c.after(5 * time.Minute)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady, nodeTaintEvicted))
				c.create(w.pod(1, "n1", pendingOnNode))
			}
			c.after(5 * time.Minute)
		},
	}
}
