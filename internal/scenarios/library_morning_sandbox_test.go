package scenarios

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// morningSandboxScenarios are a cluster scaled back up in the morning:
// every new node's pods cannot set up their network for a minute or
// two while the node's network plugin starts.
func morningSandboxScenarios() []scenario {
	return []scenario{morningSandboxSettles(), morningSandboxOneNodeStuck()}
}

// morningNodes are the nodes that join at once in the morning.
var morningNodes = []string{"m1", "m2", "m3", "m4", "m5"}

// morningApps are the Deployments scaled back up, four replicas each,
// one on each of the first four new nodes.
var morningApps = []string{"api", "web", "orders", "search", "cart",
	"billing", "media", "auth", "reports", "events"}

// morningSandboxSettles: five nodes join at once; 40 pods on four of
// them and the node agent on every one fail to set up their sandbox for
// up to 90 seconds, then all start. A morning start-up: nothing is said.
func morningSandboxSettles() scenario {
	return scenario{
		expect: expectation{
			Name: "morning-sandbox-settles",
			Description: "Five nodes join at once in the morning; 40 " +
				"pods and the node agents fail sandbox network setup " +
				"for up to 90 seconds, then all start.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildMorningSandbox(c, false) },
	}
}

// morningSandboxOneNodeStuck: the same morning, but on node m5 the
// network plugin never comes up: its pods keep failing for ten minutes
// while those of the other nodes start. One incident, for m5, never a
// page while m5 is new.
func morningSandboxOneNodeStuck() scenario {
	return scenario{
		expect: expectation{
			Name: "morning-sandbox-one-node-stuck",
			Description: "Five nodes join at once in the morning; the " +
				"pods of four start within 90 seconds, those of the " +
				"fifth keep failing sandbox network setup for ten " +
				"minutes.",
			Root: "node//m5", Tier: "notify", MaxMessages: 2,
			BootHeld: true, MustNotBlame: []string{"node//m1",
				"node//m2", "node//m3", "node//m4", "node//n0"},
			Tail: duration(5 * time.Minute),
		},
		build: func(c *cluster) { buildMorningSandbox(c, true) },
	}
}

// morningPod is a pod of the morning on node: a Deployment's replica,
// or the node agent's pod.
type morningPod struct {
	pod   *corev1.Pod
	owner *workload
	index int
	node  string
}

// buildMorningSandbox lists an old node and the agent; five nodes join,
// their pods are created and fail their sandbox. On m1–m4 they start
// within 90 seconds; with stuck, those on m5 keep failing.
func buildMorningSandbox(c *cluster, stuck bool) {
	c.list(c.node("n0", "zone-a"))
	agent := nodeAgent(c, 1, 1)
	agent.ObjectMeta = c.meta("kube-system", "csi-node")
	c.list(agent, agentPod(c, agent, "n0"))
	c.after(5 * time.Minute)
	born := c.now
	for _, name := range morningNodes {
		node := c.node(name, "zone-a")
		node.CreationTimestamp = metav1.NewTime(born)
		c.create(node)
	}
	setAgentCounts(agent, 6, 1)
	c.update(agent)
	pods := morningPods(c, agent, born)
	for round := range 3 {
		c.after(25 * time.Second)
		morningSandboxFails(c, pods, born, round == 0, "")
	}
	c.after(15 * time.Second)
	morningStart(c, pods, born, agent, stuck)
	if !stuck {
		c.after(10 * time.Minute)
		return
	}
	for range 5 {
		c.after(2 * time.Minute)
		morningSandboxFails(c, pods, born, false, "m5")
	}
}

// morningPods creates the Deployments' replicas on m1–m4, two more
// replicas of the first three Deployments on m5, and the agent's pod on
// every new node, all still creating their sandbox.
func morningPods(
	c *cluster, agent *appsv1.DaemonSet, born time.Time,
) []morningPod {
	var out []morningPod
	for a, app := range morningApps {
		nodes := morningNodes[:4]
		if a < 3 {
			nodes = morningNodes
		}
		w := c.deployment("shop", app, "registry.example.com/"+app+":3",
			int32(len(nodes)))
		w.setReady(0)
		c.list(w.objects())
		for i, node := range nodes {
			out = append(out, morningPod{pod: w.pod(i, node,
				createdAt(born), creating), owner: w, index: i,
				node: node})
		}
	}
	for _, node := range morningNodes {
		out = append(out, morningPod{pod: agentPod(c, agent, node,
			createdAt(born), creating), node: node})
	}
	for _, p := range out {
		c.create(p.pod)
	}
	return out
}

// morningSandboxFails repeats the sandbox failure of every pod still
// creating, or only of those on node when it is set.
func morningSandboxFails(
	c *cluster, pods []morningPod, born time.Time, first bool, node string,
) {
	for i, p := range pods {
		if node != "" && p.pod.Spec.NodeName != c.n(node) {
			continue
		}
		reason, message := "FailedCreatePodSandBox", sandboxRefusedMessage
		// The stuck node's plugin never initialises; elsewhere the
		// plugin's agent refuses it until it is up.
		if i%4 == 0 || p.node == "m5" {
			reason = "NetworkNotReady"
			message = "network is not ready: container runtime network " +
				"not ready: NetworkReady=false reason:NetworkPluginNotReady" +
				" message:Network plugin returns error: cni plugin not " +
				"initialized"
		}
		count := int32(2)
		if first {
			count = 1
		}
		ev := c.warningEvent(p.pod, "Pod", reason, message, "kubelet", count)
		ev.Name = fmt.Sprintf("%s.%s", p.pod.Name, reason)
		ev.InvolvedObject.UID = p.pod.UID
		c.warn(ev)
	}
}

// morningStart starts every pod but, with stuck, those on m5, and sets
// the owners' ready counts.
func morningStart(
	c *cluster, pods []morningPod, born time.Time,
	agent *appsv1.DaemonSet, stuck bool,
) {
	ready := map[*workload]int32{}
	agents := int32(1)
	for _, p := range pods {
		if stuck && p.node == "m5" {
			continue
		}
		if p.owner != nil {
			c.update(p.owner.pod(p.index, p.node, startedNow,
				createdAt(born)))
			ready[p.owner]++
			continue
		}
		c.update(agentPod(c, agent, p.node, startedNow, createdAt(born)))
		agents++
	}
	for _, app := range morningApps {
		for w, n := range ready {
			if w.deployment.Name == c.n(app) {
				w.setReady(n)
				c.update(w.objects())
			}
		}
	}
	setAgentCounts(agent, 6, agents)
	c.update(agent)
}
