package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// nodeLifecycleScenarios are nodes leaving the cluster: a spot node
// taken away, a drain a budget will not let finish, a single replica on
// a lost node, and the negative case of a node removed long before.
func nodeLifecycleScenarios() []scenario {
	return []scenario{
		spotNodeRemoved(), nodeRemovedEarlierScaleUp(),
		budgetBlocksDrain(), singleReplicaNodeLoss(),
	}
}

// insufficientCPU is the scheduler's answer when the remaining nodes
// have no room.
const insufficientCPU = "0/2 nodes are available: 2 Insufficient cpu. " +
	"preemption: 0/2 nodes are available: 2 No preemption victims " +
	"found for incoming pod."

// spotNodeRemoved: the cloud reclaims spot node n3. Its pods are gone
// and their replacements stay Pending: the two remaining nodes have no
// room. The removed node is the root, not the scheduler's "Insufficient
// cpu" or the workloads.
func spotNodeRemoved() scenario {
	return scenario{
		expect: expectation{
			Name: "spot-node-removed",
			Description: "A spot node is reclaimed; the replacements of " +
				"its pods stay Pending for lack of cpu elsewhere.",
			Root: "node//n3", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"scheduling//Insufficient cpu",
				"deployment/shop/cart", "deployment/shop/search"},
		},
		build: func(c *cluster) {
			n3 := c.node("n3", "zone-a")
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), n3)
			fleet := nodeFleet(c, "shop", []string{"cart", "search"},
				[]string{"n1", "n3"})
			c.after(time.Minute)
			c.remove(n3)
			for _, w := range fleet {
				c.remove(w.pod(1, "n3"))
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(5 * time.Second)
			for _, w := range fleet {
				c.create(w.pod(2, "", pendingUnscheduled(insufficientCPU)))
				scheduleFailedEvents(c, w, insufficientCPU, 1)
			}
			c.after(4 * time.Minute)
		},
	}
}

// nodeRemovedEarlierScaleUp: a node was scaled in long ago; now a
// Deployment scales out and its new pods find no cpu. The shortage is
// the root; the node that left half an hour earlier must not be blamed.
func nodeRemovedEarlierScaleUp() scenario {
	return scenario{
		expect: expectation{
			Name: "node-removed-earlier-scale-up",
			Description: "A Deployment scales out 20 minutes after a " +
				"node was scaled in; its new pods lack cpu.",
			Root: "scheduling//Insufficient cpu", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n3"},
		},
		build: func(c *cluster) {
			n3 := c.node("n3", "zone-a")
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), n3)
			w := c.deployment("shop", "etl", "registry.example.com/etl:5", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(time.Minute)
			c.remove(n3)
			c.after(20 * time.Minute)
			setReplicas(w, 4)
			w.setReady(2)
			c.update(w.objects())
			for i := 2; i < 4; i++ {
				c.create(w.pod(i, "", pendingUnscheduled(insufficientCPU)))
			}
			scheduleFailedEvents(c, w, insufficientCPU, 4)
		},
	}
}

// budgetBlocksDrain: n2 is drained for an upgrade; every pod leaves but
// the single ledger replica, whose budget requires one available pod.
// The drain waits forever. The budget is the root, not the node.
func budgetBlocksDrain() scenario {
	return scenario{
		expect: expectation{
			Name: "budget-blocks-drain",
			Description: "A node drain stalls: a PodDisruptionBudget " +
				"with minAvailable 1 protects a single-replica " +
				"Deployment on it.",
			Root: "poddisruptionbudget/shop/ledger", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n2", "deployment/shop/ledger"},
		},
		build: func(c *cluster) {
			n2 := c.node("n2", "zone-a")
			c.list(c.node("n1", "zone-a"), n2)
			ledger := c.deployment("shop", "ledger",
				"registry.example.com/ledger:9", 1)
			c.list(ledger.objects())
			c.list(ledger.pod(0, "n2"))
			c.list(lifecycleBudget(c, ledger, 0))
			c.after(time.Minute)
			cordoned := last(c, n2)
			cordoned.Spec.Unschedulable = true
			cordoned.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
			}}
			c.update(cordoned)
			for range 13 {
				c.after(time.Minute)
				c.update(lifecycleBudget(c, ledger, 0))
			}
		},
	}
}

// singleReplicaNodeLoss: n1 stops reporting; the only replica of the
// billing worker ran there, so the whole workload is down while every
// other workload keeps a replica elsewhere. The node is the root; the
// single replica is why it hurt, not the cause. After the eviction
// timeout the node is deleted and every pod is replaced on n2.
func singleReplicaNodeLoss() scenario {
	return scenario{
		expect: expectation{
			Name: "single-replica-node-loss",
			Description: "A node stops reporting; a single-replica " +
				"Deployment on it is fully down while replicated " +
				"workloads keep serving.",
			// Announced, spreading, then resolved with the node gone.
			Root: "node//n1", Tier: "page", MaxMessages: 3,
			MustNotBlame: []string{"deployment/billing/worker"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"cart", "search"},
				[]string{"n1", "n2"})
			worker := c.deployment("billing", "worker",
				"registry.example.com/worker:3", 1)
			c.list(worker.objects())
			c.list(worker.pod(0, "n1"))
			c.after(time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Second)
			all := append(fleet, worker)
			for _, w := range all {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(*w.deployment.Spec.Replicas - 1)
				c.update(w.objects())
			}
			// After the eviction timeout the cloud deletes the node,
			// its pods go with it and are replaced on n2.
			c.after(5 * time.Minute)
			c.remove(last(c, n1))
			for _, w := range all {
				c.remove(last(c, w.pod(0, "n1")))
				c.create(w.pod(2, "n2", startedNow))
				w.setReady(*w.deployment.Spec.Replicas)
				c.update(w.objects())
			}
			c.after(5 * time.Minute)
		},
	}
}

// lifecycleBudget is a minAvailable 1 budget over w's pods, allowing
// the given number of disruptions.
func lifecycleBudget(
	c *cluster, w *workload, allowed int32,
) *policyv1.PodDisruptionBudget {
	one := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: clusterMeta(c, "shop", "ledger", "pdb"),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: w.deployment.Spec.Template.Labels},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			CurrentHealthy: 1, DesiredHealthy: 1, ExpectedPods: 1,
			DisruptionsAllowed: allowed,
		},
	}
}
