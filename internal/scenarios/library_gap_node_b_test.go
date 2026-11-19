package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gapNodeBScenarios are nodes whose taint, network setup or clock stops
// the pods placed on them.
func gapNodeBScenarios() []scenario {
	return []scenario{
		gapNoExecuteTaint(), gapNoExecuteTolerated(),
		gapSandboxIPConflict(), gapClockSkew(),
	}
}

// gapNoExecuteTaint: operations taint n1 for maintenance with NoExecute;
// the taint manager deletes the pods that do not tolerate it and their
// replacements are ready on n3 within a minute. A deliberate taint whose
// evictions are replaced at once is maintenance, not a failure: nothing
// to tell anyone, as for a drain.
func gapNoExecuteTaint() scenario {
	return scenario{
		expect: expectation{
			Name: "node-no-execute-taint-evicts",
			Description: "A node is given a NoExecute maintenance taint; " +
				"pods of three workloads without a toleration are " +
				"deleted by the taint manager and are ready again " +
				"elsewhere within a minute.",
			Quiet: true,
		},
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			c.after(time.Minute)
			tainted := last(c, n1)
			tainted.Spec.Taints = []corev1.Taint{{
				Key: "maintenance", Value: "true",
				Effect: corev1.TaintEffectNoExecute,
			}}
			c.update(tainted)
			c.after(10 * time.Second)
			for _, w := range fleet {
				pod := w.pod(0, "n1", nodeTaintEvicted)
				c.update(pod)
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(20 * time.Second)
			for _, w := range fleet {
				c.remove(w.pod(0, "n1"))
				c.create(w.pod(2, "n3", startedNow, notReady))
			}
			c.after(30 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(2, "n3", startedNow))
				w.setReady(2)
				c.update(w.objects())
			}
			c.after(5 * time.Minute)
		},
	}
}

// gapNoExecuteTolerated: the same maintenance taint, but every pod on n1
// tolerates it, so nothing is evicted and nothing is said.
func gapNoExecuteTolerated() scenario {
	return scenario{
		expect: expectation{
			Name: "node-no-execute-taint-tolerated",
			Description: "A node is given a NoExecute taint that every " +
				"pod on it tolerates; no pod is evicted.",
			Quiet: true,
		},
		build: func(c *cluster) {
			n1, _ := gapNodeFleet(c)
			c.after(time.Minute)
			tainted := last(c, n1)
			tainted.Spec.Taints = []corev1.Taint{{
				Key: "maintenance", Value: "true",
				Effect: corev1.TaintEffectNoExecute,
			}}
			c.update(tainted)
			c.after(6 * time.Minute)
		},
	}
}

// gapSandboxIPConflict: a stale address allocation on n2 makes the CNI
// refuse new sandboxes there; new pods of two workloads wait in
// ContainerCreating on n2 while pods on other nodes start fine.
func gapSandboxIPConflict() scenario {
	return scenario{
		expect: expectation{
			Name: "node-cni-ip-conflict",
			Description: "New pods on one node stay in ContainerCreating " +
				"because the CNI reports a route that already exists " +
				"for the pod address; other nodes start pods fine.",
			Root: "node//n2", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/shop/cart"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"orders", "cart"},
				[]string{"n1", "n3"})
			c.after(time.Minute)
			message := "Failed to create pod sandbox: rpc error: code = " +
				"Unknown desc = failed to setup network for sandbox: " +
				"plugin type=\"calico\" failed (add): error adding host " +
				"side routes for interface: cali4f2a1b, error: route " +
				"(Ifindex: 12, Dst: 10.244.2.17/32, Scope: 253) already " +
				"exists for an interface other than 'cali4f2a1b'"
			for _, w := range fleet {
				*w.deployment.Spec.Replicas = 3
				w.setReady(2)
				c.update(w.objects())
				pod := w.pod(2, "n2", startedNow,
					gapPending(waiting("ContainerCreating", "")))
				c.create(pod)
				for count := int32(1); count <= 5; count++ {
					gapKubelet(c, pod, "FailedCreatePodSandBox", message,
						count)
					c.after(time.Minute)
				}
			}
		},
	}
}

// gapClockSkew: the clock of n3 runs ten minutes behind; its kubelet can
// not renew its client certificate because the CA's certificate is "not
// yet valid" by its clock, and the node goes NotReady.
func gapClockSkew() scenario {
	return scenario{
		expect: expectation{
			Name: "node-clock-skew-cert-rejected",
			Description: "A node's clock is ten minutes off; its kubelet " +
				"reports NotReady with a certificate \"is before\" " +
				"validity error while other nodes are healthy.",
			Root: "node//n3", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/shop/payments", "deployment/shop/cart"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "shop",
				[]string{"orders", "payments", "cart"}, []string{"n1", "n3"})
			c.after(2 * time.Minute)
			node := last(c, c.node("n3", "zone-a"))
			setNodeCondition(node, corev1.NodeReady, corev1.ConditionFalse,
				"KubeletNotReady", "failed to renew client certificate: "+
					"Post \"https://10.0.0.1:6443/apis/certificates.k8s."+
					"io/v1/certificatesigningrequests\": x509: "+
					"certificate has expired or is not yet valid: "+
					"current time 2026-11-19T09:52:10Z is before "+
					"2026-11-19T10:02:07Z", c.now)
			c.update(node)
			c.after(30 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(1, "n3", notReady))
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(8 * time.Minute)
		},
	}
}
