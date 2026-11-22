package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap18aScenarios: one zone, one node pool or a few replicas fail while
// the rest of the service keeps serving.
func gap18aScenarios() []scenario {
	return []scenario{
		gap18ZoneDatabase(), gap18ZoneImagePull(), gap18PoolTaint(),
		gap18PoolDiskPressure(), gap18CorruptImageCache(), gap18HotShard(),
		gap18SubPathConfig(), gap18LargeResponses(), gap18SlowPath(),
		gap18TerminatingEndpoint(),
	}
}

// g18Node is a Ready node in zone and pool.
func g18Node(c *cluster, name, zone, pool string) *corev1.Node {
	node := c.node(name, zone)
	node.Labels["pool"] = pool
	return node
}

// g18Zones lists n1..n6, two per zone of zone-a, zone-b and zone-c.
func g18Zones(c *cluster) {
	for i, zone := range []string{"zone-a", "zone-a", "zone-b", "zone-b",
		"zone-c", "zone-c"} {
		c.list(g18Node(c, "n"+itoa(i+1), zone, "pool-a"))
	}
}

// g18Deploy lists a Deployment with one ready pod on each node of place.
func g18Deploy(
	c *cluster, ns, name string, place []string, mutate func(*corev1.PodSpec),
) *workload {
	w := c.deployment(ns, name, "registry.example.com/"+name+":1.0",
		int32(len(place)))
	if mutate != nil {
		configTemplate(w, mutate)
	}
	c.list(w.objects())
	for i, node := range place {
		c.list(w.pod(i, node))
	}
	return w
}

// g18Fail updates the pods of w at the indexes in bad to state and
// settles the Deployment's ready count.
func g18Fail(
	c *cluster, w *workload, place []string, bad []int, state podState,
) {
	for _, i := range bad {
		c.update(w.pod(i, place[i], state))
	}
	g10Settle(c, w, int32(len(place)-len(bad)))
}

// gap18ZoneDatabase: two of six replicas, all on zone-b, cannot reach the
// database; the others serve.
func gap18ZoneDatabase() scenario {
	return scenario{
		expect: g2Exp("one-zone-pods-cannot-reach-database",
			"The two replicas of a Deployment on zone-b nodes fail "+
				"readiness with a database dial timeout; the four others "+
				"are Ready and the Service keeps four endpoints.",
			"zone//zone-b", "notify", "deployment/shop/api").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			g18Zones(c)
			place := []string{"n1", "n2", "n3", "n4", "n5", "n6"}
			w := g18Deploy(c, "shop", "api", place, nil)
			c.after(5 * time.Minute)
			message := "Readiness probe failed: dial tcp db:5432: i/o timeout"
			for _, i := range []int{2, 3} {
				pod := w.pod(i, place[i], notReady)
				c.update(pod)
				g10Event(c, pod, "Unhealthy", message, "kubelet", 5, c.now)
			}
			g10Settle(c, w, 4)
		},
	}
}

// gap18ZoneImagePull: the three pods landing on zone-c nodes cannot
// resolve the registry mirror; zone-a and zone-b pull fine.
func gap18ZoneImagePull() scenario {
	return scenario{
		expect: g2Exp("one-zone-image-pull-fails",
			"Three new pods of a rollout on zone-c nodes sit in "+
				"ImagePullBackOff on a mirror lookup timeout; the same "+
				"image pulls in zone-a and zone-b.",
			"zone//zone-c", "notify", "deployment/media/render").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			for i, zone := range []string{"zone-a", "zone-a", "zone-b",
				"zone-b", "zone-c", "zone-c", "zone-c"} {
				c.list(g18Node(c, "n"+itoa(i+1), zone, "pool-a"))
			}
			place := []string{"n1", "n2", "n3", "n4", "n1", "n5", "n6", "n7"}
			w := g18Deploy(c, "media", "render", place, nil)
			c.after(5 * time.Minute)
			message := "Back-off pulling image: dial tcp: lookup " +
				"mirror.zone-c.internal: i/o timeout"
			for _, i := range []int{5, 6, 7} {
				pod := w.pod(i, place[i], startedNow, waiting(
					"ImagePullBackOff", message))
				c.update(pod)
				g10Event(c, pod, "Failed", message, "kubelet", 3, c.now)
			}
			g10Settle(c, w, 5)
		},
	}
}

// gap18PoolTaint: a pool-wide NoSchedule taint leaves the pool-b replicas
// Pending.
func gap18PoolTaint() scenario {
	return scenario{
		expect: g2Exp("one-pool-new-taint-pods-pending",
			"Nodes of pool-b get a maintenance NoSchedule taint; four "+
				"replicas selecting pool-b are Pending on the untolerated "+
				"taint while pool-a workloads run.",
			"deployment/jobs/batch", "notify").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			var pool []*corev1.Node
			for i := 1; i <= 6; i++ {
				name, p := "a"+itoa(i), "pool-a"
				if i > 3 {
					name, p = "b"+itoa(i-3), "pool-b"
				}
				node := g18Node(c, name, "zone-a", p)
				c.list(node)
				if p == "pool-b" {
					pool = append(pool, node)
				}
			}
			w := c.deployment("jobs", "batch", "registry.example.com/b:1", 4)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.NodeSelector = map[string]string{"pool": "pool-b"}
			})
			w.setReady(0)
			c.list(w.objects())
			c.after(5 * time.Minute)
			for _, node := range pool {
				tainted := last(c, node)
				tainted.Spec.Taints = []corev1.Taint{{Key: "maintenance",
					Value: "true", Effect: corev1.TaintEffectNoSchedule}}
				c.update(tainted)
			}
			message := "0/6 nodes are available: 3 node(s) didn't match " +
				"Pod's node affinity/selector, 3 node(s) had untolerated " +
				"taint {maintenance: true}."
			for i := range 4 {
				c.create(w.pod(i, "", pendingUnscheduled(message)))
			}
			pendingForMinutes(c, w, message, 6)
		},
	}
}

// gap18PoolDiskPressure: every node of pool-logs raises DiskPressure and
// the kubelets evict seven pods of one Deployment.
func gap18PoolDiskPressure() scenario {
	return scenario{
		expect: g2Exp("one-pool-disk-pressure-evictions",
			"All three nodes of pool-logs raise DiskPressure; kubelets "+
				"evict seven pods of a Deployment on ephemeral storage. "+
				"Other pools are healthy.", "node//pool-logs-1", "notify").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			names := []string{"pool-logs-1", "pool-logs-2", "pool-logs-3"}
			for _, name := range names {
				c.list(g18Node(c, name, "zone-a", "pool-logs"))
			}
			c.list(g18Node(c, "pool-app-1", "zone-a", "pool-app"))
			place := []string{names[0], names[1], names[2], names[0],
				names[1], names[2], names[0]}
			w := g18Deploy(c, "logs", "shipper", place, nil)
			c.after(5 * time.Minute)
			for _, name := range names {
				g2NodeCondition(c, name, corev1.NodeDiskPressure,
					corev1.ConditionTrue, "KubeletHasDiskPressure",
					"kubelet has disk pressure")
			}
			c.after(time.Minute)
			message := "The node was low on resource: ephemeral-storage."
			g18Fail(c, w, place, []int{0, 1, 2, 3, 4, 5, 6}, evicted(message))
		},
	}
}

// gap18CorruptImageCache: one replica on node-c cannot create its
// container; the same image runs on three other nodes.
func gap18CorruptImageCache() scenario {
	return scenario{
		expect: g2Exp("one-replica-corrupt-image-cache-node",
			"One replica on node-c stays in CreateContainerError "+
				"\"failed to resolve rootfs\"; the same image runs on "+
				"three other nodes.", "node//node-c", "notify").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			place := []string{"node-a", "node-b", "node-d", "node-c"}
			for _, name := range place {
				c.list(c.node(name, "zone-a"))
			}
			w := g18Deploy(c, "shop", "web", place, nil)
			c.after(5 * time.Minute)
			message := "failed to resolve rootfs: layer does not exist"
			pod := w.pod(3, "node-c", startedNow, waiting(
				"CreateContainerError", message))
			c.update(pod)
			g10Event(c, pod, "Failed", "Error: "+message, "kubelet", 4, c.now)
			g10Settle(c, w, 3)
		},
	}
}

// gap18HotShard: one of three cache replicas is OOM-killed again and
// again; the others never restart.
func gap18HotShard() scenario {
	return scenario{
		expect: g2Exp("one-replica-oom-hot-shard",
			"One of three 512Mi cache replicas is OOMKilled six times in "+
				"forty minutes; the others restart zero times.",
			"deployment/shop/cache", "notify", "node//n2").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			place := []string{"n1", "n2", "n3"}
			for _, name := range place {
				c.list(c.node(name, "zone-a"))
			}
			w := g18Deploy(c, "shop", "cache", place,
				func(spec *corev1.PodSpec) {
					spec.Containers[0].Resources.Limits = corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("512Mi")}
				})
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.after(7 * time.Minute)
				g18Fail(c, w, place, []int{1}, crashLoop(137, "OOMKilled", "",
					restarts))
			}
		},
	}
}

// gap18SubPathConfig: two Deployments share a ConfigMap; the one that
// mounts it through subPath never sees the edit.
func gap18SubPathConfig() scenario {
	return scenario{
		expect: g2Exp("some-replicas-old-config-subpath",
			"A shared ConfigMap is edited; pods mounting it as a whole "+
				"volume see the change, the legacy Deployment that mounts "+
				"one file through subPath keeps the old content and fails.",
			"deployment/shop/api-legacy", "notify",
			"configmap/shop/api-config", "deployment/shop/api").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			cm := configMap(c, "shop", "api-config",
				map[string]string{"app.conf": "mode=old"})
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), cm)
			mount := func(sub string) func(*corev1.PodSpec) {
				return func(spec *corev1.PodSpec) {
					spec.Volumes = []corev1.Volume{{Name: "conf",
						VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.
							ConfigMapVolumeSource{LocalObjectReference: corev1.
							LocalObjectReference{Name: cm.Name}}}}}
					spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
						Name: "conf", MountPath: "/etc/app", SubPath: sub}}
				}
			}
			place := []string{"n1", "n2"}
			g18Deploy(c, "shop", "api", place, mount(""))
			legacy := g18Deploy(c, "shop", "api-legacy", place,
				mount("app.conf"))
			c.after(5 * time.Minute)
			edited := last(c, cm)
			edited.Data["app.conf"] = "mode=new"
			c.update(edited)
			c.after(5 * time.Minute)
			for restarts := int32(1); restarts <= 4; restarts++ {
				g18Fail(c, legacy, place, []int{0, 1}, crashLoop(1, "Error",
					"config: key mode=old is no longer accepted", restarts))
				c.after(2 * time.Minute)
			}
		},
	}
}

// gap18LargeResponses: nothing Kubernetes reports names the cause.
func gap18LargeResponses() scenario {
	e := g2Exp("one-path-large-responses-time-out",
		"Responses over about 1400 bytes from pods on one node time out "+
			"after a CNI MTU change; pods are Ready, probes pass and no "+
			"Warning events exist.", rootUnknown, "digest").budget(1).
		tail(10 * time.Minute)
	return scenario{expect: e, build: func(c *cluster) {
		g10Fleet(c, "shop", "export", 3, []string{"node-a", "node-b",
			"node-d"}, nil)
		c.after(10 * time.Minute)
	}}
}

// gap18SlowPath: one path is slow; health checks, endpoints and events
// are all clean.
func gap18SlowPath() scenario {
	return scenario{
		expect: g2Quiet("one-path-slow-dependency-health-ok",
			"Only /report on a Deployment exceeds the ingress timeout; "+
				"/healthz passes, nothing restarts, no warnings.").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			g10Fleet(c, "shop", "api", 3, gap10Nodes, nil)
			c.after(10 * time.Minute)
		},
	}
}

// gap18TerminatingEndpoint: during a rolling update the Service keeps a
// terminating endpoint for three seconds after its pod is killed.
func gap18TerminatingEndpoint() scenario {
	return scenario{
		expect: g2Exp("intermittent-resets-from-terminating-endpoint",
			"A rolling update of a Deployment resets connections for "+
				"eight seconds per replaced pod: old pods exit 143 at "+
				"once while the EndpointSlice still lists them as "+
				"terminating and serving.", "deployment/shop/backend",
			"digest").budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			place := []string{"n1", "n2", "n3"}
			for _, name := range place {
				c.list(c.node(name, "zone-a"))
			}
			w := g18Deploy(c, "shop", "backend", place, nil)
			old := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n2"),
				w.pod(2, "n3")}
			c.after(5 * time.Minute)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/backend:2"
			})
			c.update(w.deployment)
			c.create(rs)
			for i, node := range place {
				c.create(w.pod(i, node, startedNow))
				gone := old[i].DeepCopy()
				gone.Status.ContainerStatuses[0].State = corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{ExitCode: 143,
						Reason:     "Error",
						FinishedAt: metav1.NewTime(c.now)}}
				c.update(gone)
				c.create(g18Terminating(c, gone))
				c.after(3 * time.Second)
				c.remove(gone)
				c.after(10 * time.Second)
			}
			g10Settle(c, w, 3)
		},
	}
}

// g18Terminating is an EndpointSlice that still lists pod as serving
// while terminating.
func g18Terminating(c *cluster, pod *corev1.Pod) *discoveryv1.EndpointSlice {
	meta := clusterMeta(c, "shop", "backend-"+pod.Name, "eps")
	meta.Labels = map[string]string{
		discoveryv1.LabelServiceName: c.n("backend")}
	yes, no := true, false
	return &discoveryv1.EndpointSlice{
		ObjectMeta: meta, AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.9"},
			TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: pod.Name,
				Namespace: pod.Namespace},
			Conditions: discoveryv1.EndpointConditions{Ready: &no,
				Serving: &yes, Terminating: &yes}}},
	}
}
