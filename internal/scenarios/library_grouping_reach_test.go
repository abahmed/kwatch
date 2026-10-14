package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// reachScenarios are outages whose first message must reach the right
// teams, name the failing part itself, and read the same on updates.
func reachScenarios() []scenario {
	return []scenario{
		nodeLostOwnerRouted(), cniAgentCrashQuoted(),
		corednsDownNoProbe(), dependencyClusterIPDial(),
	}
}

// nodeLostOwnerRouted: n1 stops reporting while workloads of four
// teams run on it. A node has no owner and no namespace, so each
// team's route must be found from what runs on the node.
func nodeLostOwnerRouted() scenario {
	return scenario{
		expect: expectation{
			Name: "node-lost-owner-routed",
			Description: "A node is lost; its pods belong to four " +
				"namespaces owned by four teams.",
			Root: "node//n1", Tier: "page", MaxMessages: 4,
			Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			for i, ns := range groupNamespaces {
				space := &corev1.Namespace{ObjectMeta: c.meta("", ns)}
				space.Labels = map[string]string{
					kube.OwnerKey: fmt.Sprintf("team-%d", i)}
				c.list(space)
			}
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			fleet := groupFleet(c, 2, []string{"n1", "n2"})
			c.after(time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(1)
				c.update(w.deployment, w.replicaSet)
			}
			c.after(10 * time.Minute)
			nodeRecover(c, n1)
			for _, w := range fleet {
				c.update(w.pod(0, "n1"))
				w.setReady(2)
				c.update(w.deployment, w.replicaSet)
			}
		},
	}
}

// cniAgentCrashQuoted: the network agent's pod on n1 crash-loops, so
// new pods there cannot get an address. The node message must quote
// the agent's own last words, not only the symptoms.
func cniAgentCrashQuoted() scenario {
	return scenario{
		expect: expectation{
			Name: "cni-agent-crash-quoted",
			Description: "The CNI agent crash-loops on one node and " +
				"new pods there cannot be networked.",
			Root: "node//n1", Tier: "notify", MaxMessages: 4,
			Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			n1, n2 := c.node("n1", "zone-a"), c.node("n2", "zone-a")
			ds := nodeAgent(c, 2, 2)
			c.list(n1, n2, ds, agentPod(c, ds, "n1"), agentPod(c, ds, "n2"))
			fleet := groupFleet(c, 2, []string{"n1", "n2"})
			c.after(5 * time.Minute)
			setAgentCounts(ds, 2, 1)
			c.update(ds)
			c.update(agentPod(c, ds, "n1", crashLoop(1, "Error",
				"failed to initialize ipamd: no IP addresses available",
				4)))
			c.after(30 * time.Second)
			for _, w := range fleet {
				pod := w.pod(5, "n1", waiting("ContainerCreating", ""))
				pod.CreationTimestamp = metav1.NewTime(c.now)
				c.create(pod)
				ev := c.warningEvent(pod, "Pod", "FailedCreatePodSandBox",
					"Failed to create pod sandbox: rpc error: code = "+
						"Unknown desc = failed to setup network for "+
						"sandbox: plugin type=\"aws-cni\" failed (add): "+
						"failed to assign an IP address to container",
					"kubelet", 3)
				ev.InvolvedObject.UID = pod.UID
				c.warn(ev)
			}
			c.after(6 * time.Minute)
			for n := int32(5); n < 9; n++ {
				failedDaemonPod(c, ds, "n1", n)
			}
			c.after(4 * time.Minute)
		},
	}
}

// corednsDownNoProbe: CoreDNS stops being ready and clients fail to
// resolve names, but kwatch's own DNS probe has no result. The client
// errors alone make cluster DNS the root, and CoreDNS folds under it.
func corednsDownNoProbe() scenario {
	return scenario{
		expect: expectation{
			Name: "coredns-down-no-probe",
			Description: "CoreDNS has no ready pods and clients in " +
				"four namespaces log name-resolution errors; there " +
				"is no DNS probe signal.",
			Root: "cluster-dns//cluster-dns", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//n1", "node//n2"},
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
			dns := c.deployment("kube-system", "coredns",
				"registry.k8s.io/coredns/coredns:v1.11.1", 2)
			c.list(dns.objects())
			c.list(dns.pod(0, "n1"), dns.pod(1, "n2"))
			fleet := groupFleet(c, 2, []string{"n1", "n2"})
			c.after(time.Minute)
			dns.setReady(0)
			c.update(dns.objects())
			c.update(dns.pod(0, "n1", notReady), dns.pod(1, "n2", notReady))
			c.after(30 * time.Second)
			for r := int32(1); r <= 5; r++ {
				for _, w := range fleet {
					clusterDNSVictims(c, w,
						"db."+w.deployment.Namespace+".svc.cluster.local", r)
				}
				c.after(40 * time.Second)
			}
		},
	}
}

// dependencyClusterIPDial: clients in four namespaces dial the ClusterIP
// of the postgres Service and get refused, because postgres has no
// ready endpoints. The address is in the cluster, not an external one.
func dependencyClusterIPDial() scenario {
	return scenario{
		expect: expectation{
			Name: "dependency-clusterip-dial",
			Description: "Clients crash dialling a Service's ClusterIP " +
				"while the Service has no ready endpoints.",
			Root: "deployment/data/postgres", Tier: "notify",
			MaxMessages: 6, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
			db := c.deployment("data", "postgres",
				"registry.example.com/postgres:15", 1)
			c.list(db.objects())
			dbp := db.pod(0, "n1")
			c.list(dbp)
			svc := clusterService(c, "data", "postgres", 5432)
			svc.Spec.ClusterIP = "10.96.4.7"
			svc.Spec.ClusterIPs = []string{"10.96.4.7"}
			c.list(svc, clusterSlice(c, "data", "postgres", dbp))
			fleet := groupFleet(c, 2, []string{"n1", "n2"})
			c.after(time.Minute)
			db.setReady(0)
			c.update(db.objects())
			c.update(db.pod(0, "n1", notReady))
			c.update(clusterSlice(c, "data", "postgres"))
			c.after(20 * time.Second)
			msg := "Error: connect ECONNREFUSED " +
				"postgres.data.svc.cluster.local:5432 (dial tcp " +
				"10.96.4.7:5432: connect: connection refused)"
			for r := int32(1); r <= 5; r++ {
				for _, w := range fleet {
					c.update(
						w.pod(0, "n1", crashLoop(1, "Error", msg, r)),
						w.pod(1, "n2", crashLoop(1, "Error", msg, r)))
					w.setReady(0)
					c.update(w.objects())
				}
				c.after(40 * time.Second)
			}
		},
	}
}
