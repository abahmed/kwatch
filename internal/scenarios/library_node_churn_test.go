package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeChurnScenarios are the failures that come with nodes joining and
// leaving: a node-level DaemonSet whose pods fail on the node that goes
// away and on the node that replaces it, and a pressure stall on a node
// that has just started its pods. Each has a twin on a settled node,
// where the same signal is a real finding.
func nodeChurnScenarios() []scenario {
	return []scenario{
		daemonSetNodeReplaced(), daemonSetSettledNodeFails(),
		pressureSpikeOnFreshNode(), pressureSustainedOnSettledNode(),
	}
}

// nodeAgent builds the DaemonSet of a node-level agent.
func nodeAgent(c *cluster, desired, ready int32) *appsv1.DaemonSet {
	labels := map[string]string{"app": "aws-node"}
	ds := &appsv1.DaemonSet{
		ObjectMeta: c.meta("kube-system", "aws-node"),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/cni:1"),
		},
	}
	ds.Generation, ds.Status.ObservedGeneration = 1, 1
	setAgentCounts(ds, desired, ready)
	return ds
}

func setAgentCounts(ds *appsv1.DaemonSet, desired, ready int32) {
	ds.Status.DesiredNumberScheduled = desired
	ds.Status.CurrentNumberScheduled = desired
	ds.Status.UpdatedNumberScheduled = desired
	ds.Status.NumberReady = ready
	ds.Status.NumberAvailable = ready
	ds.Status.NumberUnavailable = desired - ready
}

// agentPod is the DaemonSet's pod on node.
func agentPod(
	c *cluster, ds *appsv1.DaemonSet, node string, states ...podState,
) *corev1.Pod {
	yes := true
	name := ds.Name + "-" + node
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ds.Namespace, UID: types.UID(name),
			Labels:            ds.Spec.Template.Labels,
			CreationTimestamp: metav1.NewTime(c.start.Add(-time.Hour)),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "DaemonSet", Name: ds.Name,
				UID: ds.UID, Controller: &yes,
			}},
		},
		Spec: *ds.Spec.Template.Spec.DeepCopy(),
	}
	pod.Spec.NodeName = c.n(node)
	running(c, pod)
	for _, state := range states {
		state(c, pod)
	}
	return pod
}

// failedDaemonPod is the DaemonSet controller's event for a pod it
// found failed on node.
func failedDaemonPod(
	c *cluster, ds *appsv1.DaemonSet, node string, count int32,
) {
	c.warn(c.warningEvent(ds, "DaemonSet", "FailedDaemonPod",
		"Found failed daemon pod "+ds.Namespace+"/"+ds.Name+"-"+node+
			" on node "+c.n(node)+", will try to kill it",
		"daemonset-controller", count))
}

// daemonSetNodeReplaced: a node is cordoned and removed, and the node
// that replaces it joins. The DaemonSet controller finds the agent pod
// failed on both, and the new node's pod is not ready for two minutes.
// That is the nodes' lifecycle; nothing is said.
func daemonSetNodeReplaced() scenario {
	return scenario{
		expect: expectation{
			Name: "daemonset-node-replaced",
			Description: "A node is replaced; its node-level DaemonSet " +
				"pods fail on the leaving node and on the new one.",
			Quiet: true,
		},
		build: func(c *cluster) {
			n1, n2 := c.node("n1", "zone-a"), c.node("n2", "zone-a")
			ds := nodeAgent(c, 2, 2)
			p1, p2 := agentPod(c, ds, "n1"), agentPod(c, ds, "n2")
			c.list(n1, n2, ds, p1, p2)
			c.after(10 * time.Minute)

			n3 := c.node("n3", "zone-a")
			n3.CreationTimestamp = metav1.NewTime(c.now)
			c.create(n3)
			leaving := last(c, n2)
			leaving.Spec.Unschedulable = true
			c.update(leaving)
			failedDaemonPod(c, ds, "n2", 1)
			c.after(time.Minute)
			c.remove(n2, p2)
			setAgentCounts(ds, 2, 1)
			c.update(ds)
			c.create(agentPod(c, ds, "n3", startedNow, notReady))
			failedDaemonPod(c, ds, "n3", 1)
			c.after(2 * time.Minute)
			c.update(agentPod(c, ds, "n3", startedNow))
			setAgentCounts(ds, 2, 2)
			c.update(ds)
			c.after(10 * time.Minute)
		},
	}
}

// daemonSetSettledNodeFails: the agent pod of a node that has been up
// for a day keeps failing. That is the DaemonSet's problem, and it is
// reported as a notification: FailedDaemonPod is a Warning, not a
// digest reason.
func daemonSetSettledNodeFails() scenario {
	return scenario{
		expect: expectation{
			Name: "daemonset-settled-node-fails",
			Description: "A node-level DaemonSet pod keeps failing on " +
				"a node that has been up for a day.",
			Root: "daemonset/kube-system/aws-node", Tier: "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			n1, n2 := c.node("n1", "zone-a"), c.node("n2", "zone-a")
			ds := nodeAgent(c, 2, 1)
			c.list(n1, n2, ds, agentPod(c, ds, "n1"),
				agentPod(c, ds, "n2", notReady))
			for i := int32(1); i <= 4; i++ {
				c.after(2 * time.Minute)
				failedDaemonPod(c, ds, "n2", i)
			}
			c.after(10 * time.Minute)
		},
	}
}

// nodePSI is the share of the last minute a node's tasks stalled on
// memory, as the kubelet reports it.
func nodePSI(c *cluster, node string, percent float64) {
	c.emit(inventory.Observation{
		Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
		Entity: inventory.CoreID(kube.KindNode, "", c.n(node)),
		Attributes: map[string]inventory.Value{
			kube.AttrMemoryPSI: inventory.Number(percent),
		},
	})
}

// pressureSpikeOnFreshNode: half an hour after it joined, a node's
// tasks stall on memory twice for two minutes while its pods start.
// Neither spike lasts; nothing is said.
func pressureSpikeOnFreshNode() scenario {
	return scenario{
		expect: expectation{
			Name: "pressure-spike-fresh-node",
			Description: "Memory stall spikes of two minutes on a node " +
				"that joined half an hour ago.",
			Quiet: true,
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			n1.CreationTimestamp = metav1.NewTime(c.start)
			c.list(n1)
			c.after(30 * time.Minute)
			for range 2 {
				for range 2 {
					nodePSI(c, "n1", 45)
					c.after(time.Minute)
				}
				for range 5 {
					nodePSI(c, "n1", 3)
					c.after(time.Minute)
				}
			}
		},
	}
}

// pressureSustainedOnSettledNode: the tasks of a node that has been up
// for a day stall on memory for twelve minutes. Workloads slow down for
// that long; it is reported to the digest.
func pressureSustainedOnSettledNode() scenario {
	return scenario{
		expect: expectation{
			Name: "pressure-sustained-settled-node",
			Description: "A memory stall of twelve minutes on a node " +
				"that has been up for a day.",
			Root: "node//n1", Tier: "digest", MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			for range 12 {
				nodePSI(c, "n1", 45)
				c.after(time.Minute)
			}
			c.after(5 * time.Minute)
		},
	}
}
