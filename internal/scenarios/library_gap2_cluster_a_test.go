package scenarios

import (
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap2ClusterAScenarios are shared services of the cluster, DNS, the API
// server's reachability and shared storage, whose fault shows in many
// workloads.
func gap2ClusterAScenarios() []scenario {
	return []scenario{
		gap2APIServerEgress(), gap2PauseImage(), gap2NFSDown(),
		gap2CorefileEdit(), gap2NodeLocalDNS(), gap2DNSStorm(),
	}
}

// gap2Spread lists one Deployment per "namespace/name" with a replica on
// each of nodes.
func gap2Spread(c *cluster, apps, nodes []string) []*workload {
	var out []*workload
	for _, app := range apps {
		namespace, name, _ := strings.Cut(app, "/")
		w := c.deployment(namespace, name,
			"registry.example.com/"+name+":1.0", int32(len(nodes)))
		c.list(w.objects())
		for i, node := range nodes {
			c.list(w.pod(i, node))
		}
		out = append(out, w)
	}
	return out
}

// gap2CrashPods restarts the replicas of fleet listed in affected, which
// run on nodes[i], with message; the rest of each workload stays ready.
func gap2CrashPods(
	c *cluster, fleet []*workload, nodes []string, affected []int,
	message string, restarts int32,
) {
	for _, w := range fleet {
		for _, i := range affected {
			c.update(w.pod(i, nodes[i], crashLoop(1, "Error", message,
				restarts)))
		}
		w.setReady(int32(len(nodes) - len(affected)))
		c.update(w.objects())
	}
}

// gap2APIServerEgress: a new egress NetworkPolicy allows only DNS for the
// pods of the sync controller; its calls to the API server's ClusterIP
// time out. The API server answers kwatch's own probe.
func gap2APIServerEgress() scenario {
	return scenario{
		expect: g2Exp("networkpolicy-blocks-apiserver-access",
			"An egress NetworkPolicy that allows only DNS is applied to "+
				"a controller; its calls to the API server's address "+
				"time out while the API server answers its probe.",
			"networkpolicy/shop/dns-only-egress", "notify",
			"apiserver//kube-apiserver", "deployment/shop/sync",
			"node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			sync := c.deployment("shop", "sync",
				"registry.example.com/sync:3", 2)
			c.list(sync.objects())
			c.list(sync.pod(0, "n1"), sync.pod(1, "n2"))
			c.probe(kube.APIServer, "")
			c.after(5 * time.Minute)
			c.create(dnsOnlyEgress(c, sync))
			c.after(time.Minute)
			message := "Get \"https://10.96.0.1:443/api/v1/namespaces/" +
				"shop/configmaps\": dial tcp 10.96.0.1:443: i/o timeout"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(sync.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), sync.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				sync.setReady(0)
				c.update(sync.objects())
				c.probe(kube.APIServer, "")
				c.after(time.Minute)
			}
		},
	}
}

// gap2PauseImage: n4 was added to the pool with a runtime configured to
// pull the sandbox (pause) image from a registry the node cannot reach;
// pods of three namespaces placed there never get a sandbox.
func gap2PauseImage() scenario {
	return scenario{
		expect: g2Exp("node-pause-image-pull-fails",
			"Pods of three namespaces placed on one node never get a "+
				"sandbox: FailedCreatePodSandBox says the sandbox image "+
				"cannot be pulled; pods on other nodes run.",
			"node//n4", "notify", "deployment/shop/web",
			"deployment/billing/invoices", "deployment/media/transcode").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n4", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, []string{"n1"})
			c.after(5 * time.Minute)
			message := "Failed to create pod sandbox: rpc error: code = " +
				"Unknown desc = failed to get sandbox image " +
				"\"registry.k8s.io/pause:3.9\": failed to pull image " +
				"\"registry.k8s.io/pause:3.9\": failed to pull and " +
				"unpack image \"registry.k8s.io/pause:3.9\": failed to " +
				"resolve reference \"registry.k8s.io/pause:3.9\": " +
				"failed to do request: Head \"https://registry.k8s.io/" +
				"v2/pause/manifests/3.9\": dial tcp 34.96.108.209:443: " +
				"i/o timeout"
			for _, w := range fleet {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
			}
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n4", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedCreatePodSandBox", message,
						round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap2NFSDown: the NFS server that three namespaces mount is
// unreachable; the pods that were restarted since cannot mount it and
// wait in ContainerCreating. The server is outside the cluster.
func gap2NFSDown() scenario {
	return scenario{
		expect: g2Exp("nfs-server-unreachable-three-namespaces",
			"Pods of three namespaces that mount the same NFS server "+
				"stay in ContainerCreating with FailedMount \"Connection "+
				"timed out\".",
			"unknown", "notify", "node//n1", "node//n2",
			"deployment/shop/web", "deployment/billing/invoices",
			"deployment/media/transcode").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, []string{"n1"})
			c.after(5 * time.Minute)
			message := "MountVolume.SetUp failed for volume \"shared\" : " +
				"mount failed: exit status 32 Mounting command: mount " +
				"Mounting arguments: -t nfs nfs.example.internal:" +
				"/export/shared /var/lib/kubelet/pods/6a1c/volumes/" +
				"kubernetes.io~nfs/shared Output: mount.nfs: Connection " +
				"timed out"
			for _, w := range fleet {
				old := w.pod(0, "n1")
				c.remove(old)
				w.setReady(0)
				c.update(w.objects())
			}
			for round := int32(1); round <= 6; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n2", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedMount", message, round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap2CorefileEdit: someone edits the Corefile ConfigMap and the new
// configuration makes CoreDNS exit at start; lookups fail in every
// namespace and the DNS probe fails.
func gap2CorefileEdit() scenario {
	return scenario{
		expect: g2Exp("coredns-corefile-edit-breaks-dns",
			"The CoreDNS Corefile ConfigMap is edited and CoreDNS "+
				"crash-loops on the new configuration; lookups fail in "+
				"several namespaces and the DNS probe fails.",
			"configmap/kube-system/coredns", "page", "node//n1",
			"node//n2", "deployment/shop/cart",
			"deployment/billing/invoices"),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
			cm := configMap(c, "kube-system", "coredns", map[string]string{
				"Corefile": ".:53 {\n    errors\n    health\n    " +
					"kubernetes cluster.local in-addr.arpa ip6.arpa\n    " +
					"forward . /etc/resolv.conf\n    cache 30\n}\n"})
			dns := c.deployment("kube-system", "coredns",
				"registry.k8s.io/coredns/coredns:v1.11.1", 2)
			configTemplate(dns, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{configVolume("config-volume",
					c.n("coredns"))}
			})
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:3.1", 2)
			invoices := c.deployment("billing", "invoices",
				"registry.example.com/invoices:1.8", 2)
			c.list(cm)
			for _, w := range []*workload{dns, cart, invoices} {
				c.list(w.objects())
				c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			}
			c.probe(kube.ClusterDNS, "")
			c.after(10 * time.Minute)
			edited := last(c, cm)
			edited.Data["Corefile"] = ".:53 {\n    errors\n    health\n" +
				"    kubernetes cluster.local in-addr.arpa ip6.arpa\n" +
				"    forward . 127.0.0.53\n    loop\n    cache 30\n}\n"
			c.update(edited)
			c.after(time.Minute)
			loop := "[FATAL] plugin/loop: Loop (127.0.0.1:41738 -> " +
				":53) detected for zone \".\", see https://coredns.io/" +
				"plugins/loop#troubleshooting"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(dns.pod(0, "n1", crashLoop(1, "Error", loop,
					restarts)), dns.pod(1, "n2", crashLoop(1, "Error",
					loop, restarts)))
				dns.setReady(0)
				c.update(dns.objects())
				c.probe(kube.ClusterDNS, "lookup kubernetes.default.svc."+
					"cluster.local on 10.96.0.10:53: read udp 10.244.1."+
					"7:41234->10.96.0.10:53: i/o timeout")
				if restarts >= 2 {
					clusterDNSVictims(c, cart, "db.shop.svc.cluster.local",
						restarts-1)
					clusterDNSVictims(c, invoices,
						"payments-api.billing.svc.cluster.local",
						restarts-1)
				}
				c.after(30 * time.Second)
			}
		},
	}
}

// gap2NodeLocalDNS: the node-local DNS cache crash-loops on n1 and n2,
// where the pods' resolver points at it; three workloads in three
// namespaces fail lookups there while their replicas on n3 work.
func gap2NodeLocalDNS() scenario {
	return scenario{
		expect: g2Exp("node-local-dns-down-on-two-nodes",
			"The node-local DNS cache DaemonSet crash-loops on two "+
				"nodes; workloads of three namespaces fail name "+
				"lookups there while their replicas on a third node "+
				"work.",
			"daemonset/kube-system/node-local-dns", "notify",
			"node//n1", "node//n2", "cluster-dns//cluster-dns",
			"deployment/shop/web", "deployment/billing/invoices",
			"deployment/media/transcode").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			nodes := []string{"n1", "n2", "n3"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			ds := g2Daemon(c, "kube-system", "node-local-dns", nodes...)
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, nodes)
			c.probe(kube.ClusterDNS, "")
			c.after(2 * time.Minute)
			agent := "[FATAL] listen tcp 169.254.20.10:8080: bind: address " +
				"already in use"
			lookup := "dial tcp: lookup db on 169.254.20.10:53: read udp " +
				"10.244.1.5:48762->169.254.20.10:53: i/o timeout"
			for restarts := int32(1); restarts <= 6; restarts++ {
				setAgentCounts(ds, 3, 1)
				c.update(ds, agentPod(c, ds, "n1", crashLoop(1, "Error",
					agent, restarts)), agentPod(c, ds, "n2",
					crashLoop(1, "Error", agent, restarts)))
				if restarts >= 2 {
					gap2CrashPods(c, fleet, nodes, []int{0, 1}, lookup,
						restarts-1)
				}
				c.probe(kube.ClusterDNS, "")
				c.after(time.Minute)
			}
		},
	}
}

// gap2DNSStorm: a crawler retries a failing lookup in a tight loop,
// restarting dozens of times; CoreDNS is OOM-killed under the load and
// the lookups of other namespaces time out. The looping workload is the
// cause; CoreDNS shows the effect.
func gap2DNSStorm() scenario {
	e := g2Exp("dns-storm-from-looping-workload",
		"A crawler crash-loops on a failing lookup and floods CoreDNS; "+
			"CoreDNS is OOM-killed and lookups of other namespaces time "+
			"out.", "deployment/batch/crawler", "notify").budget(3).
		tail(10 * time.Minute)
	e.Ambiguous = true
	return scenario{
		expect: e,
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			dns := c.deployment("kube-system", "coredns",
				"registry.k8s.io/coredns/coredns:v1.11.1", 2)
			crawler := c.deployment("batch", "crawler",
				"registry.example.com/crawler:5", 3)
			c.list(dns.objects())
			c.list(dns.pod(0, "n1"), dns.pod(1, "n2"))
			c.list(crawler.objects())
			c.list(crawler.pod(0, "n1"), crawler.pod(1, "n2"),
				crawler.pod(2, "n1"))
			victims := gap2Spread(c, []string{"shop/web",
				"billing/invoices"}, []string{"n1", "n2"})
			c.probe(kube.ClusterDNS, "")
			c.after(time.Minute)
			loop := "Get \"https://feed.partner.example/v1/items\": dial " +
				"tcp: lookup feed.partner.example on 10.96.0.10:53: no " +
				"such host"
			lookup := "dial tcp: lookup db on 10.96.0.10:53: read udp " +
				"10.244.1.5:48762->10.96.0.10:53: i/o timeout"
			for round := int32(1); round <= 6; round++ {
				for i := range 3 {
					c.update(crawler.pod(i, []string{"n1", "n2", "n1"}[i],
						crashLoop(1, "Error", loop, round*9)))
				}
				crawler.setReady(0)
				c.update(crawler.objects())
				if round >= 2 {
					c.update(dns.pod(0, "n1", crashLoop(137, "OOMKilled", "",
						round-1)), dns.pod(1, "n2", crashLoop(137,
						"OOMKilled", "", round-1)))
					dns.setReady(0)
					c.update(dns.objects())
					gap2CrashPods(c, victims, []string{"n1", "n2"},
						[]int{0}, lookup, round-1)
					c.probe(kube.ClusterDNS, "lookup kubernetes.default."+
						"svc.cluster.local on 10.96.0.10:53: i/o timeout")
				}
				c.after(time.Minute)
			}
		},
	}
}
