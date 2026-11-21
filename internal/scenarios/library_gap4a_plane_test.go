package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap4aPlaneScenarios are probes, restarts and the control plane's own
// controllers: how a story reads when the cluster's machinery misbehaves.
func gap4aPlaneScenarios() []scenario {
	return []scenario{
		gap4aGRPCProbeTLS(), gap4aFastBackoff(), gap4aAllNodesDisrupted(),
		gap4aLingeringEvicted(), gap4aLeaderLost(), gap4aNoisyClient(),
	}
}

// gap4aGRPCProbeTLS: a release adds a gRPC liveness probe to a server that
// only speaks TLS; the probe fails every time and the kubelet restarts a
// healthy container.
func gap4aGRPCProbeTLS() scenario {
	return g2Release(g2Exp("release-grpc-liveness-probe-tls-server",
		"A release adds a plain gRPC liveness probe to a TLS-only "+
			"server; the probe fails and the kubelet restarts the "+
			"container while the application logs no error.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].LivenessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{GRPC: &corev1.GRPCAction{
					Port: 9090}}, PeriodSeconds: 10, FailureThreshold: 3}
		},
		func(c *cluster, w *workload) {
			gapRounds(c, w, 6, 45*time.Second, func(round int32) podState {
				return crashLoop(143, "Error", "", round)
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "Unhealthy", "Liveness probe failed: "+
					"rpc error: code = Unavailable desc = connection "+
					"error: desc = \"error reading server preface: "+
					"EOF\"", round)
			})
		})
}

// gap4aFastBackoff: the cluster uses a reduced crash-loop back-off; a bad
// release restarts every twenty seconds, so the counter climbs far faster
// than usual. It is still one problem.
func gap4aFastBackoff() scenario {
	return g2Release(g2Exp("release-crash-loop-reduced-backoff",
		"A bad release crash-loops under a reduced restart back-off: "+
			"the restart count reaches 90 in minutes.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].Env = []corev1.EnvVar{{Name: "MODE",
				Value: "fast"}}
		},
		func(c *cluster, w *workload) {
			gapRounds(c, w, 8, 20*time.Second, func(round int32) podState {
				return crashLoop(1, "Error", "FATAL: config key \"MODE\" "+
					"has no value \"fast\"", round*12)
			}, nil)
		})
}

// gap4aAllNodesDisrupted: every node stops reporting at once, as in a
// partition between the nodes and the API; the node lifecycle controller
// halts evictions, so the pods stay and are never replaced.
func gap4aAllNodesDisrupted() scenario {
	return scenario{
		expect: g2Exp("all-nodes-lost-evictions-halted",
			"Every node stops reporting in the same minute; no pod is "+
				"evicted or replaced because the controller halts "+
				"evictions when all zones are disrupted.",
			"node//n1", "page", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(15 * time.Minute),
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			c.after(2 * time.Minute)
			nodeLose(c, n1)
			nodeLose(c, c.node("n2", "zone-a"))
			nodeLose(c, c.node("n3", "zone-a"))
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady), w.pod(1, "n2", notReady))
				w.setReady(0)
				c.update(w.objects())
			}
			c.after(12 * time.Minute)
		},
	}
}

// gap4aLingeringEvicted: after a disk incident that ended hours ago, forty
// Evicted pods are still listed in two namespaces; every workload runs.
func gap4aLingeringEvicted() scenario {
	return scenario{
		expect: g2Quiet("evicted-pods-linger-after-recovery-quiet",
			"Forty Evicted pods from an incident hours ago are still "+
				"listed in two namespaces while every workload runs on "+
				"healthy nodes."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/orders", "billing/ledger"},
				[]string{"n1", "n2"})
			for i := range 40 {
				w := fleet[i%2]
				pod := w.pod(10+i, "n1", evictedSince(
					c.now.Add(-3*time.Hour-30*time.Minute),
					"The node was low on resource: ephemeral-storage."))
				c.list(pod)
			}
			c.after(30 * time.Minute)
		},
	}
}

// gap4aLeaderLost: etcd answers slowly; the scheduler cannot renew its
// lease and loses leadership, and pods created in that minute stay
// Pending with no event until it is leader again.
func gap4aLeaderLost() scenario {
	return scenario{
		expect: gap4aRevises(g2Exp("scheduler-leader-lost-after-etcd-slow",
			"etcd requests time out; the scheduler loses its lease and "+
				"new pods stay Pending without events until it is "+
				"leader again.",
			"etcd//etcd", "page", "deployment/shop/cart", "deployment/shop/search",
			"deployment/billing/invoices", "node//n1", "node//n2")),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := controlPlaneFleet(c)
			c.probe(kube.Etcd, "")
			c.probe(kube.Scheduler, "")
			c.after(time.Minute)
			slow := "[-]etcd failed: etcdserver: request timed out"
			for round := range 4 {
				c.probe(kube.Etcd, slow)
				c.probe(kube.Scheduler, fmt.Sprintf("leader lease not "+
					"renewed for %dm5s", round+1))
				if round == 1 {
					for _, w := range fleet {
						setReplicas(w, 3)
						w.setReady(2)
						c.update(w.objects())
						c.create(w.pod(2, "", notScheduled))
					}
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap4aNoisyClient: an inventory operator lists every object in the
// cluster each minute; the API server answers other clients' requests
// with 429 and three controllers' pods exit.
func gap4aNoisyClient() scenario {
	e := g2Exp("apf-noisy-client-starves-others",
		"One operator's list-heavy client saturates its priority level; "+
			"other clients sharing the level get 429 responses and "+
			"their pods exit.", "deployment/ops/inventory-sync", "notify",
		"node//n1", "node//n2").budget(3).tail(10 * time.Minute)
	e.Ambiguous = true
	return scenario{expect: e, build: func(c *cluster) {
		c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
		op := c.deployment("ops", "inventory-sync",
			"registry.example.com/inventory-sync:2", 1)
		c.list(op.objects())
		c.list(op.pod(0, "n1"))
		fleet := gap2Spread(c, []string{"shop/orders", "billing/ledger",
			"media/thumbs"}, []string{"n1", "n2"})
		c.after(5 * time.Minute)
		message := "failed to list *v1.ConfigMap: the server has " +
			"received too many requests and has asked us to try again " +
			"later (get configmaps)"
		for restarts := int32(1); restarts <= 6; restarts++ {
			gap2CrashPods(c, fleet, []string{"n1", "n2"}, []int{0, 1},
				message, restarts)
			c.after(time.Minute)
		}
	}}
}

// gap4aRevises lets the first message name a real intermediate cause.
func gap4aRevises(e expectation) expectation {
	e.RevisesRoot = true
	return e
}
