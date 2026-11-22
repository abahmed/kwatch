package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap5bClusterScenarios are DNS blips, lost nodes, control-plane failover
// and shared storage, and the states around them that must stay quiet.
func gap5bClusterScenarios() []scenario {
	return []scenario{
		gap5bCoreDNSPodRemoved(), gap5bConntrackResets(),
		gap5bMemoryOvercommit(), gap5bProbeOnTerminatingQuiet(),
		gap5bPodsOnMissingNode(), gap5bSpreadSkewQuiet(),
		gap5bControlPlaneFailover(), gap5bEtcdWALCorrupt(),
		gap5bNFSProviderRestarts(),
	}
}

// gap5bCoreDNSPodRemoved: one of two CoreDNS pods is replaced in a
// rollout; two callers see a lookup time out once and are fine again.
func gap5bCoreDNSPodRemoved() scenario {
	return scenario{
		expect: g2Exp("coredns-pod-removed-lookups-stall-briefly",
			"One CoreDNS pod is replaced in a rollout; lookups of two "+
				"workloads stall for the resolver timeout and recover "+
				"within two minutes.",
			"deployment/kube-system/coredns", "notify", "node//n1",
			"node//n2", "deployment/shop/cart",
			"deployment/billing/invoices").tail(10 * time.Minute),
		build: func(c *cluster) {
			dns, fleet := gap5aDNS(c)
			c.after(10 * time.Minute)
			c.remove(dns.pod(0, "n1"))
			c.create(dns.pod(2, "n1", startedNow))
			for restarts := int32(1); restarts <= 2; restarts++ {
				gap2CrashPods(c, fleet, []string{"n1", "n2"}, []int{0},
					"dial tcp: lookup db.shop.svc.cluster.local on "+
						"10.96.0.10:53: read udp 10.244.1.5:48762->"+
						"10.96.0.10:53: i/o timeout", restarts)
				c.after(time.Minute)
			}
			for _, w := range fleet {
				c.update(w.pod(0, "n1"))
				w.setReady(2)
				c.update(w.objects())
			}
			c.after(10 * time.Minute)
		},
	}
}

// gap5bConntrackResets: now and then a pod of a busy service restarts on
// "connection reset by peer"; nothing in the cluster explains it.
func gap5bConntrackResets() scenario {
	return scenario{
		expect: g2Exp("sporadic-connection-reset-no-cause",
			"A busy service's pods restart now and then on \"connection "+
				"reset by peer\" with no event, condition or change "+
				"anywhere in the cluster.",
			"unknown", "digest", "node//n1", "node//n2").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 3)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n1"))
			for round := int32(1); round <= 4; round++ {
				c.after(20 * time.Minute)
				i := int(round) % 3
				node := []string{"n1", "n2", "n1"}[i]
				c.update(w.pod(i, node, crashLoop(1, "Error", "read tcp "+
					"10.244.1.9:43210->10.96.14.7:5432: read: connection "+
					"reset by peer", 1)))
				w.setReady(2)
				c.update(w.objects())
				c.after(30 * time.Second)
				c.update(w.pod(i, node, gap2Restarted(1)))
				w.setReady(3)
				c.update(w.objects())
			}
		},
	}
}

// gap5bMemoryOvercommit: pods with small requests and large real use fill
// a node's memory; it reports pressure, kills several workloads and goes
// NotReady.
func gap5bMemoryOvercommit() scenario {
	return scenario{
		expect: g2Exp("node-memory-starved-by-low-requests",
			"Pods whose requests are far below their use starve a "+
				"node of memory; it reports MemoryPressure, several "+
				"workloads are OOMKilled and it goes NotReady.",
			"node//n1", "page", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			c.after(5 * time.Minute)
			g2NodeCondition(c, "n1", corev1.NodeMemoryPressure,
				corev1.ConditionTrue, "KubeletHasInsufficientMemory",
				"kubelet has insufficient memory available")
			for restarts := int32(1); restarts <= 2; restarts++ {
				for _, w := range fleet {
					c.update(w.pod(0, "n1", crashLoop(137, "OOMKilled", "",
						restarts)))
					w.setReady(1)
					c.update(w.objects())
				}
				c.after(time.Minute)
			}
			nodeLose(c, n1)
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
			}
			c.after(6 * time.Minute)
		},
	}
}

// gap5bProbeOnTerminatingQuiet: one probe failure is recorded for a pod a
// moment after it is deleted; the Deployment replaces it at once.
func gap5bProbeOnTerminatingQuiet() scenario {
	return scenario{
		expect: g2Quiet("probe-failure-on-terminating-pod-quiet",
			"One readiness probe failure appears for a pod just after "+
				"it is deleted; its replacement is ready within a "+
				"minute."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			pod := w.pod(0, "n1")
			deleting(&pod.ObjectMeta, c.now)
			c.update(pod)
			gapKubelet(c, pod, "Unhealthy", "Readiness probe failed: "+
				"Get \"http://10.244.1.9:8080/ready\": dial tcp "+
				"10.244.1.9:8080: connect: connection refused", 1)
			c.create(w.pod(2, "n1", startedNow, notReady))
			c.after(20 * time.Second)
			c.remove(w.pod(0, "n1"))
			c.update(w.pod(2, "n1", startedNow))
			c.after(10 * time.Minute)
		},
	}
}

// gap5bPodsOnMissingNode: an autoscaler deleted n9; the pods that were
// bound to it are still in the API, Running or Terminating, and nothing
// replaces them.
func gap5bPodsOnMissingNode() scenario {
	return scenario{
		expect: g2Exp("pods-bound-to-node-that-no-longer-exists",
			"Pods of two workloads still read Running or Terminating on "+
				"a node whose object was deleted; their controllers do "+
				"not replace them.",
			"node//n9", "notify", "deployment/shop/orders",
			"deployment/shop/payments").tail(15 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"orders", "payments"},
				[]string{"n9", "n9"})
			c.after(5 * time.Minute)
			for _, w := range fleet {
				pod := w.pod(0, "n9", notReady)
				deleting(&pod.ObjectMeta, c.now.Add(-30*time.Minute))
				c.update(pod)
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(20 * time.Minute)
		},
	}
}

// gap5bSpreadSkewQuiet: after a rollout three replicas sit on two nodes
// and none on the third; they are all ready.
func gap5bSpreadSkewQuiet() scenario {
	return scenario{
		expect: g2Quiet("spread-skew-after-rollout-quiet",
			"After a rollout the three replicas of a Deployment with a "+
				"spread constraint sit on two of three nodes; all are "+
				"ready."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 3)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.TopologySpreadConstraints = []corev1.
					TopologySpreadConstraint{{MaxSkew: 1,
					TopologyKey:       corev1.LabelHostname,
					WhenUnsatisfiable: corev1.DoNotSchedule}}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n3"))
			c.after(5 * time.Minute)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{{Name: "V",
					Value: "2"}}
			})
			c.update(w.deployment)
			c.create(rs)
			c.create(w.pod(0, "n1", startedNow), w.pod(1, "n1", startedNow),
				w.pod(2, "n2", startedNow))
			c.after(10 * time.Minute)
		},
	}
}

// gap5bControlPlaneFailover: the API server's load balancer fails over;
// every kubelet holds a dead connection for six minutes, every node reads
// Unknown, then all return together without a drain or a reboot.
func gap5bControlPlaneFailover() scenario {
	e := g2Exp("control-plane-failover-nodes-unknown-then-recover",
		"Every node reads Ready=Unknown for six minutes after a "+
			"control-plane failover and all return together; no drain "+
			"or reboot is involved.", "unknown", "page",
		"deployment/shop/orders", "deployment/shop/payments",
		"deployment/shop/cart").budget(3).tail(10 * time.Minute)
	e.Ambiguous = true
	return scenario{expect: e, build: func(c *cluster) {
		n1, fleet := gapNodeFleet(c)
		c.probe(kube.APIServer, "")
		c.after(5 * time.Minute)
		nodes := []*corev1.Node{n1, c.node("n2", "zone-a"),
			c.node("n3", "zone-a")}
		for _, node := range nodes {
			nodeLose(c, node)
		}
		c.probe(kube.APIServer, "Get \"https://10.96.0.1:443/readyz\": "+
			"dial tcp 10.96.0.1:443: i/o timeout")
		c.after(40 * time.Second)
		for _, w := range fleet {
			c.update(w.pod(0, "n1", notReady), w.pod(1, "n2", notReady))
			w.setReady(0)
			c.update(w.objects())
		}
		c.after(6 * time.Minute)
		for _, node := range nodes {
			g2NodeCondition(c, node.Name[:2], corev1.NodeReady,
				corev1.ConditionTrue, "KubeletReady",
				"kubelet is posting ready status")
		}
		c.probe(kube.APIServer, "")
		for _, w := range fleet {
			c.update(w.pod(0, "n1"), w.pod(1, "n2"))
			w.setReady(2)
			c.update(w.objects())
		}
		c.after(5 * time.Minute)
	}}
}

// gap5bEtcdWALCorrupt: after a reboot of the control-plane node etcd
// cannot replay its log and crash-loops; the API server behind it
// follows.
func gap5bEtcdWALCorrupt() scenario {
	return scenario{
		expect: g2Exp("etcd-wal-corrupt-after-reboot",
			"After a control-plane node reboots, etcd crash-loops on a "+
				"corrupt write-ahead log and the API server follows.",
			"etcd//etcd", "page", "apiserver//kube-apiserver", "node//n1").
			budget(3),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			etcd := c.deployment("kube-system", "etcd",
				"registry.k8s.io/etcd:3.5.12", 1)
			controlPlaneLabels(etcd, "etcd")
			api := c.deployment("kube-system", "kube-apiserver",
				"registry.k8s.io/kube-apiserver:v1.30.2", 1)
			for _, w := range []*workload{etcd, api} {
				c.list(w.objects())
				c.list(w.pod(0, "n1"))
			}
			c.probe(kube.Etcd, "")
			c.probe(kube.APIServer, "")
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(etcd.pod(0, "n1", crashLoop(2, "Error", "wal: "+
					"crc mismatch", restarts)))
				etcd.setReady(0)
				c.update(etcd.objects())
				c.update(api.pod(0, "n1", crashLoop(1, "Error", "dial tcp "+
					"127.0.0.1:2379: connect: connection refused",
					restarts)))
				api.setReady(0)
				c.update(api.objects())
				c.probe(kube.Etcd, "[-]etcd failed: dial tcp 127.0.0.1:2379: "+
					"connect: connection refused")
				c.probe(kube.APIServer, "")
				c.after(time.Minute)
			}
		},
	}
}

// gap5bNFSProviderRestarts: the pod that serves an NFS volume to three
// namespaces is replaced; the consumers keep running unready, and every
// replacement of theirs fails to mount.
func gap5bNFSProviderRestarts() scenario {
	return scenario{
		expect: g2Exp("in-cluster-nfs-server-restarts-consumers-hang",
			"The pod that serves an NFS volume is replaced; consumers "+
				"in three namespaces stop responding and their "+
				"replacements fail FailedMount.",
			"deployment/storage/nfs-server", "notify", "node//n1",
			"deployment/shop/web", "deployment/billing/invoices",
			"deployment/media/transcode").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			nfs := c.deployment("storage", "nfs-server",
				"registry.example.com/nfs-server:2", 1)
			c.list(nfs.objects())
			c.list(nfs.pod(0, "n2"))
			c.list(scheduleClass(c, "nfs", "Immediate"))
			c.list(clusterService(c, "storage", "nfs-server", 2049))
			var fleet []*workload
			for _, app := range [][2]string{{"shop", "web"},
				{"billing", "invoices"}, {"media", "transcode"}} {
				fleet = append(fleet, gap5bNFSConsumer(c, app[0], app[1]))
			}
			c.after(10 * time.Minute)
			c.remove(nfs.pod(0, "n2"))
			c.create(nfs.pod(1, "n2", startedNow))
			message := "MountVolume.SetUp failed for volume \"shared\" : " +
				"mount failed: exit status 32 Output: mount.nfs: " +
				"Stale file handle"
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(0)
				c.update(w.objects())
			}
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n1", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedMount", message, round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap5bNFSConsumer is a one-replica workload on n1 that mounts a claim
// bound to a volume served by the nfs-server Service.
func gap5bNFSConsumer(c *cluster, namespace, name string) *workload {
	claim := storageBoundClaim(c, namespace, "shared", "nfs", "10Gi",
		"nfs-"+namespace)
	volume := storageVolume(c, claim, "nfs-"+namespace)
	volume.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{
		NFS: &corev1.NFSVolumeSource{
			Server: "nfs-server.storage.svc.cluster.local",
			Path:   "/exports"},
	}
	w := c.deployment(namespace, name,
		"registry.example.com/"+name+":1.0", 1)
	storageMount(w, claim.Name)
	c.list(claim, volume)
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	return w
}
