package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// rcaAgentScenarios are node agents that fail beside pods whose own
// errors have another cause, and a storage plugin missing from a node.
func rcaAgentScenarios() []scenario {
	return []scenario{
		agentWithHealthyPeers(), proxyCrashCSINamesOtherDriver(),
		csiPluginMissingOnNode(),
	}
}

// agentWithHealthyPeers: a log shipper crash-loops on n1 while two
// unrelated applications, each with a healthy replica on n2 and with
// healthy neighbours on n1, crash on different panics. The agent serves
// every pod on n1, and most of them are fine: it is not their cause.
func agentWithHealthyPeers() scenario {
	return scenario{
		expect: expectation{
			Name: "node-agent-with-healthy-peers",
			Description: "A node agent crash-loops on a node where two " +
				"of six workloads crash on their own panics and the " +
				"rest run.",
			Root:        "deployment/shop/api",
			OtherRoots:  []string{"deployment/shop/web"},
			Tier:        "notify",
			MaxMessages: 5,
			Unrelated:   []string{"daemonset/observability/logship"},
		},
		build: func(c *cluster) {
			ds := logshipAgent(c)
			var fleet []*workload
			for _, name := range []string{"api", "web", "cart", "search",
				"pay", "mail"} {
				w := c.deployment("shop", name,
					"registry.example.com/"+name+":2", 2)
				c.list(w.objects())
				c.list(w.pod(0, "n1"), w.pod(1, "n2"))
				fleet = append(fleet, w)
			}
			c.after(10 * time.Minute)
			setAgentCounts(ds, 2, 1)
			c.update(ds)
			panics := map[string]string{
				"api": "panic: runtime error: index out of range [3] " +
					"with length 3",
				"web": "panic: assignment to entry in nil map",
			}
			for round := int32(1); round <= 5; round++ {
				c.update(agentPod(c, ds, "n1", crashLoop(1, "Error",
					"failed to open /var/log/pods: permission denied",
					round)))
				for _, w := range fleet[:2] {
					c.update(w.pod(0, "n1", crashLoop(2, "Error",
						panics[w.deployment.Name], round)))
					w.setReady(1)
					c.update(w.objects())
				}
				c.after(time.Minute)
			}
		},
	}
}

// proxyCrashCSINamesOtherDriver: kube-proxy crash-loops on n1, and the
// api pod there cannot mount its volume because a storage driver is not
// registered. The error names a driver kube-proxy has nothing to do
// with, so kube-proxy is not blamed for the mount.
func proxyCrashCSINamesOtherDriver() scenario {
	return scenario{
		expect: expectation{
			Name: "node-agent-crash-csi-error-names-other-driver",
			Description: "kube-proxy crash-loops on a node where a " +
				"pod's volume cannot be mounted because a storage " +
				"driver is not registered.",
			Root: rootUnknown, Tier: "notify", MaxMessages: 4,
			Unrelated: []string{"daemonset/kube-system/kube-proxy"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			proxy := rcaAgent(c, "kube-system", "kube-proxy",
				[]string{"n1", "n2"})
			csi := csiNodePlugin(c)
			c.list(csi, agentPod(c, csi, "n1"), agentPod(c, csi, "n2"))
			api := c.deployment("shop", "api",
				"registry.example.com/api:4", 1)
			c.list(api.objects())
			c.after(10 * time.Minute)
			setAgentCounts(proxy, 2, 1)
			c.update(proxy)
			for round := int32(1); round <= 4; round++ {
				c.update(agentPod(c, proxy, "n1", crashLoop(1, "Error",
					"failed to execute iptables-restore: exit status 4",
					round*3)))
				rcaMountPending(c, api, "n1", round, csiNotRegistered(c))
				c.after(time.Minute)
			}
			c.after(5 * time.Minute)
		},
	}
}

// rcaMountPending is the api pod waiting for a volume the kubelet of
// node cannot mount, with the kubelet's event of that round.
func rcaMountPending(
	c *cluster, w *workload, node string, round int32, message string,
) {
	pod := w.pod(0, node, waiting("ContainerCreating", ""))
	pod.CreationTimestamp = metav1.NewTime(c.now)
	pod.Status.Phase = corev1.PodPending
	if round == 1 {
		c.create(pod)
	} else {
		c.update(pod)
	}
	c.warn(c.warningEvent(pod, "Pod", "FailedMount", message, "kubelet",
		round*3))
}

// csiPluginMissingOnNode: the CSI node plugin DaemonSet runs on n1 and
// n2 but has no pod on n3, which its tolerations do not cover. The api
// pod on n3 cannot mount its volume: the driver is not registered
// there. The DaemonSet is the root, not the node and not the pod.
func csiPluginMissingOnNode() scenario {
	return scenario{
		expect: expectation{
			Name: "csi-node-plugin-missing-on-node",
			Description: "A pod cannot mount its volume because the " +
				"CSI node plugin DaemonSet has no pod on its node.",
			Root: "daemonset/kube-system/ebs-csi-node", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//n3"},
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			ds := csiNodePlugin(c)
			c.list(ds, agentPod(c, ds, "n1"), agentPod(c, ds, "n2"))
			api := c.deployment("shop", "api",
				"registry.example.com/api:4", 1)
			c.list(api.objects())
			c.after(10 * time.Minute)
			for round := int32(1); round <= 4; round++ {
				rcaMountPending(c, api, "n3", round, csiNotRegistered(c))
				c.after(time.Minute)
			}
			c.after(5 * time.Minute)
		},
	}
}
