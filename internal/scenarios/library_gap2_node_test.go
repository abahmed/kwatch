package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap2NodeScenarios are faults of one node that its pods show.
func gap2NodeScenarios() []scenario {
	return []scenario{
		gap2HostPortTaken(), gap2HostPathMissing(), gap2RuntimeNotReady(),
		gap2InTreeVolume(), gap2KubeletRestartsAll(),
		gap2AdmissionOutOfCPU(), gap2AdmissionSingleReject(),
		gap2ImageGCFailed(), gap2ConntrackFull(), gap2MaxMapCount(),
	}
}

// gap2NodeCrash restarts replica idx of every workload on node with
// message, once a minute for rounds rounds; the other replica is ready.
func gap2NodeCrash(
	c *cluster, fleet []*workload, node string, idx int, message string,
	rounds int32,
) {
	for round := int32(1); round <= rounds; round++ {
		for _, w := range fleet {
			c.update(w.pod(idx, node, crashLoop(1, "Error", message,
				round)))
			w.setReady(1)
			c.update(w.objects())
		}
		c.after(time.Minute)
	}
}

// gap2HostPortTaken: a hostNetwork gateway runs on n1 and n2; on n1 an
// exporter outside Kubernetes already listens on its port 9100, so that
// replica exits at every start while the one on n2 serves.
func gap2HostPortTaken() scenario {
	return scenario{
		expect: g2Exp("hostnetwork-port-taken-on-node",
			"A hostNetwork Deployment's replica on one node exits with "+
				"\"address already in use\" because another process of "+
				"that node holds the port; its replica on the other "+
				"node serves.",
			"node//n1", "notify", "deployment/shop/edge", "node//n2").
			budget(3),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "edge", "registry.example.com/edge:3",
				2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.HostNetwork = true
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(2 * time.Minute)
			for round := int32(1); round <= 6; round++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error",
					"listen tcp 0.0.0.0:9100: bind: address already in "+
						"use", round)))
				w.setReady(1)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap2HostPathMissing: a Deployment mounts the hostPath /data/cache with
// type Directory; a node that joined the pool recently lacks it, so the
// replica scheduled there waits in ContainerCreating.
func gap2HostPathMissing() scenario {
	return scenario{
		expect: g2Exp("hostpath-directory-missing-on-node",
			"A replica placed on a newly added node waits in "+
				"ContainerCreating with FailedMount \"hostPath type "+
				"check failed\" while the replicas on the other nodes "+
				"run.",
			"node//n3", "notify", "node//n1", "node//n2",
			"deployment/shop/cache").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "cache", "registry.example.com/cache:4",
				2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				kind := corev1.HostPathDirectory
				spec.Volumes = []corev1.Volume{{Name: "cache",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/data/cache", Type: &kind}}}}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(5 * time.Minute)
			*w.deployment.Spec.Replicas = 3
			w.setReady(2)
			c.update(w.objects())
			created := c.now
			for round := int32(1); round <= 6; round++ {
				pod := w.pod(2, "n3", createdAt(created),
					gapPending(waiting("ContainerCreating", "")))
				c.update(pod)
				gapKubelet(c, pod, "FailedMount", "MountVolume.SetUp "+
					"failed for volume \"cache\" : hostPath type check "+
					"failed: /data/cache is not a directory", round)
				c.after(time.Minute)
			}
		},
	}
}

// gap2RuntimeNotReady: after a node upgrade the kubelet of n1 cannot talk
// to the container runtime it was pointed at and reports NotReady; the
// pods on it go unready.
func gap2RuntimeNotReady() scenario {
	return scenario{
		expect: g2Exp("node-runtime-not-ready-after-upgrade",
			"A node upgrade leaves the kubelet unable to reach its "+
				"container runtime; the node reports NotReady with "+
				"\"container runtime not ready\" and its pods go "+
				"unready.",
			"node//n1", "page", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(5 * time.Minute),
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			c.after(time.Minute)
			down := last(c, n1)
			setNodeCondition(down, corev1.NodeReady, corev1.ConditionFalse,
				"KubeletNotReady", "container runtime not ready: "+
					"RuntimeReady=false reason:RuntimeNotReady message:"+
					"failed to get runtime status: rpc error: code = "+
					"Unimplemented desc = unknown service runtime.v1."+
					"RuntimeService", c.now)
			c.update(down)
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(1)
				c.update(w.objects())
			}
		},
	}
}

// gap2InTreeVolume: after a cluster upgrade removed the in-tree
// awsElasticBlockStore plugin, a Deployment that still names such a
// volume cannot start its replacement pod.
func gap2InTreeVolume() scenario {
	return scenario{
		expect: g2Exp("upgrade-in-tree-volume-plugin-removed",
			"After a cluster upgrade a pod that still uses an in-tree "+
				"awsElasticBlockStore volume cannot mount it: FailedMount "+
				"\"no volume plugin matched\".",
			"deployment/shop/ledger", "notify", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "ledger", "registry.example.com/ledger:7",
				1)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{{Name: "data",
					VolumeSource: corev1.VolumeSource{
						AWSElasticBlockStore: &corev1.
							AWSElasticBlockStoreVolumeSource{
							VolumeID: "vol-0a1b2c3d", FSType: "ext4"}}}}
			})
			c.list(w.objects())
			old := w.pod(0, "n1")
			c.list(old)
			c.after(10 * time.Minute)
			c.remove(old)
			w.setReady(0)
			c.update(w.objects())
			created := c.now
			for round := int32(1); round <= 6; round++ {
				pod := w.pod(1, "n1", createdAt(created),
					gapPending(waiting("ContainerCreating", "")))
				c.update(pod)
				gapKubelet(c, pod, "FailedMount", "MountVolume."+
					"NewMounter initialization failed for volume "+
					"\"data\" : no volume plugin matched name: "+
					"kubernetes.io/aws-ebs", round)
				c.after(time.Minute)
			}
		},
	}
}

// gap2Restarted marks container 0 as restarted count times, last by
// exit code 137, and ready again.
func gap2Restarted(count int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = count
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 137, Reason: "Error",
				StartedAt:  metav1.NewTime(c.now.Add(-time.Hour)),
				FinishedAt: metav1.NewTime(c.now),
			},
		}
	}
}

// gap2KubeletRestartsAll: n1's kubelet is upgraded and, as its container
// hash changes, it restarts every container of the node at once: three
// workloads in three seconds, all ready again within half a minute.
func gap2KubeletRestartsAll() scenario {
	return scenario{
		expect: g2Exp("node-kubelet-upgrade-restarts-all-pods",
			"A kubelet upgrade changes the container hash and restarts "+
				"every container of the node at once; three workloads "+
				"restart together and are ready again within a minute.",
			"node//n1", "digest", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			_, fleet := gapNodeFleet(c)
			c.after(5 * time.Minute)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", gap2Restarted(1), notReady))
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(25 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", gap2Restarted(1)))
				w.setReady(2)
				c.update(w.objects())
			}
		},
	}
}

// gap2AdmissionOutOfCPU: the scheduler keeps binding the replicas of a
// Deployment to n2, whose kubelet rejects each with OutOfcpu because its
// own accounting shows no room; the ReplicaSet replaces every one and the
// failed pods pile up.
func gap2AdmissionOutOfCPU() scenario {
	return scenario{
		expect: g2Exp("kubelet-admission-out-of-cpu-pileup",
			"Pods bound to one node are rejected by its kubelet with "+
				"OutOfcpu; the ReplicaSet replaces each and the failed "+
				"pods pile up.",
			"node//n2", "notify", "deployment/batch/render").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("batch", "render", "registry.example.com/r:2", 3)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"), w.pod(2, "n1"))
			c.after(2 * time.Minute)
			message := "Pod was rejected: Node didn't have enough " +
				"resource: cpu, requested: 2000, used: 3100, capacity: 4000"
			*w.deployment.Spec.Replicas = 6
			w.setReady(3)
			c.update(w.objects())
			for i := 3; i < 9; i++ {
				pod := w.pod(i, "n2", g2Failed("OutOfcpu", message))
				c.create(pod)
				gapKubelet(c, pod, "OutOfcpu", message, 1)
				c.after(time.Minute)
			}
		},
	}
}

// gap2AdmissionSingleReject: one pod is rejected by a kubelet with
// OutOfcpu in a scheduling race; its replacement lands on another node
// and is ready within a minute. Nothing stays wrong.
func gap2AdmissionSingleReject() scenario {
	return scenario{
		expect: g2Quiet("kubelet-admission-single-reject-replaced",
			"One pod is rejected by a kubelet with OutOfcpu after a "+
				"scheduling race; its replacement runs on another node "+
				"within a minute."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("batch", "render", "registry.example.com/r:2", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(2 * time.Minute)
			message := "Pod was rejected: Node didn't have enough " +
				"resource: cpu, requested: 2000, used: 3100, capacity: 4000"
			*w.deployment.Spec.Replicas = 3
			w.setReady(2)
			c.update(w.objects())
			pod := w.pod(2, "n2", g2Failed("OutOfcpu", message))
			c.create(pod)
			gapKubelet(c, pod, "OutOfcpu", message, 1)
			c.after(20 * time.Second)
			c.remove(pod)
			c.create(w.pod(3, "n1", startedNow, notReady))
			c.after(25 * time.Second)
			c.update(w.pod(3, "n1", startedNow))
			w.setReady(3)
			c.update(w.objects())
			c.after(5 * time.Minute)
		},
	}
}

// gap2ImageGCFailed: the image filesystem of n1 is full, the kubelet's
// image garbage collection finds nothing it may delete, the node reports
// DiskPressure and starts evicting.
func gap2ImageGCFailed() scenario {
	return scenario{
		expect: g2Exp("node-image-gc-failed-disk-pressure",
			"A node's image filesystem is full; image garbage "+
				"collection reports ImageGCFailed, the node has "+
				"DiskPressure and its pods are evicted.",
			"node//n1", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			c.after(time.Minute)
			node := g2NodeCondition(c, "n1", corev1.NodeDiskPressure,
				corev1.ConditionTrue, "KubeletHasDiskPressure",
				"kubelet has disk pressure")
			g2Event(c, n1, "Node", "ImageGCFailed", "failed to garbage "+
				"collect required amount of images. Attempted to free "+
				"6442450944 bytes, but only found 0 bytes eligible to "+
				"free.", "kubelet", 3)
			g2Event(c, node, "Node", "FreeDiskSpaceFailed", "Failed to "+
				"garbage collect required amount of images. Attempted "+
				"to free 6442450944 bytes, but only found 0 bytes "+
				"eligible to free.", "kubelet", 3)
			c.after(time.Minute)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", evicted("The node was low on "+
					"resource: ephemeral-storage. Threshold quantity: "+
					"10%, available: 1%.")))
				w.setReady(1)
				c.update(w.objects())
			}
			c.after(5 * time.Minute)
		},
	}
}
