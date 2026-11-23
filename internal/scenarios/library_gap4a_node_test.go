package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// gap4aNodeScenarios are runtime and kubelet features a pod needs and a
// node does not offer, and the node warnings that report it.
func gap4aNodeScenarios() []scenario {
	return []scenario{
		gap4aRuntimeClassMissing(), gap4aUserNamespaceMount(),
		gap4aImageVolumeMissing(), gap4aPodPidsLimit(),
		gap4aHugepagesGone(), gap4aInvalidDiskCapacity(),
		gap4aAllocatableEnforcement(),
	}
}

// gap4aRuntimeClassMissing: a sandboxed Deployment runs on n1 and n2; its
// third replica lands on n3, whose runtime has no handler for the class.
func gap4aRuntimeClassMissing() scenario {
	return scenario{
		expect: g2Exp("node-runtime-class-handler-missing",
			"A pod with a RuntimeClass lands on the one node whose "+
				"runtime has no such handler; it stays in "+
				"ContainerCreating with FailedCreatePodSandBox.",
			"node//n3", "notify", "deployment/shop/sandboxed").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "sandboxed",
				"registry.example.com/sandboxed:2", 2)
			class := "gvisor"
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.RuntimeClassName = &class
			})
			c.list(builtinObject(c, "node.k8s.io/v1", "RuntimeClass", "",
				class, map[string]any{}, nil))
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			*w.deployment.Spec.Replicas = 3
			w.setReady(2)
			c.update(w.objects())
			message := "Failed to create pod sandbox: rpc error: code = " +
				"Unknown desc = failed to get sandbox runtime: no " +
				"runtime for \"gvisor\" is configured"
			created := c.now
			for round := int32(1); round <= 6; round++ {
				pod := w.pod(2, "n3", createdAt(created),
					gapPending(waiting("ContainerCreating", "")))
				c.update(pod)
				gapKubelet(c, pod, "FailedCreatePodSandBox", message, round)
				c.after(time.Minute)
			}
		},
	}
}

// gap4aUserNamespaceMount: a release sets hostUsers false for pods that
// mount a volume on a file system without idmapped mounts.
func gap4aUserNamespaceMount() scenario {
	return g2Release(g2Exp("release-user-namespace-volume-unsupported",
		"A release runs the pods in a user namespace (hostUsers false) "+
			"while their volume's file system lacks idmapped mounts; "+
			"the new pod stays in ContainerCreating with FailedMount.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.HostUsers = boolPtr(false)
			spec.Volumes = []corev1.Volume{{Name: "shared",
				VolumeSource: corev1.VolumeSource{NFS: &corev1.
					NFSVolumeSource{Server: "nfs.example.internal",
					Path: "/export/shared"}}}}
		},
		g2Mount("MountVolume.SetUp failed for volume \"shared\" : mount "+
			"failed: idmapped mounts are not supported on this "+
			"filesystem"))
}

// gap4aImageVolumeMissing: a release mounts an OCI image as a volume with
// a tag that does not exist.
func gap4aImageVolumeMissing() scenario {
	return g2Release(g2Exp("release-image-volume-not-found",
		"A release mounts an image volume whose tag does not exist; the "+
			"new pod cannot start and the kubelet reports the pull "+
			"failing with \"not found\".",
		"deployment/shop/api", "notify", "node//n1", "node//n2",
		"registry//registry.example.com"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Volumes = []corev1.Volume{{Name: "model",
				VolumeSource: corev1.VolumeSource{Image: &corev1.
					ImageVolumeSource{Reference: "registry.example.com/" +
					"models/bert:v9"}}}}
		},
		g2Wait("ContainerCreating", "Failed", "Failed to pull image "+
			"\"registry.example.com/models/bert:v9\": rpc error: code = "+
			"NotFound desc = failed to pull and unpack image "+
			"\"registry.example.com/models/bert:v9\": not found"))
}

// gap4aPodPidsLimit: the worker forks more processes than the kubelet's
// per-pod PID limit allows; every pod exits at the same point, and the
// nodes have PIDs to spare.
func gap4aPodPidsLimit() scenario {
	return scenario{
		expect: g2Exp("workload-hits-pod-pids-limit",
			"A worker crashes with \"Resource temporarily unavailable\" "+
				"when it forks past the per-pod PID limit while its "+
				"nodes report no pressure.",
			"deployment/shop/worker", "notify", "node//n1", "node//n2").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "worker",
				"registry.example.com/worker:6", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(5 * time.Minute)
			message := "java.lang.OutOfMemoryError: unable to create native " +
				"thread: possibly out of memory or process/resource " +
				"limits reached (pthread_create failed: Resource " +
				"temporarily unavailable)"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), w.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap4aHugepagesGone: n2 reboots without its hugepages boot setting; its
// allocatable hugepages fall to zero and the restarted database pod
// cannot be placed again.
func gap4aHugepagesGone() scenario {
	return scenario{
		expect: g2Exp("node-hugepages-allocatable-lost",
			"A node reboots without its hugepages setting; its "+
				"allocatable hugepages fall to zero and the pod that "+
				"needs them is Pending \"Insufficient hugepages\". The "+
				"rebooted node's lost allocatable explains it, not "+
				"the scheduling reason alone.",
			"node//n2", "notify", "deployment/shop/db").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			huge := corev1.ResourceName(corev1.ResourceHugePagesPrefix + "2Mi")
			n2 := c.node("n2", "zone-a")
			n2.Status.Allocatable[huge] = resource.MustParse("4Gi")
			c.list(c.node("n1", "zone-a"), n2)
			w := c.deployment("shop", "db", "registry.example.com/db:5", 1)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].Resources.Limits = corev1.ResourceList{
					huge:                  resource.MustParse("2Gi"),
					corev1.ResourceMemory: resource.MustParse("2Gi")}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n2"))
			c.after(10 * time.Minute)
			rebooted := last(c, n2)
			rebooted.Status.Allocatable[huge] = resource.MustParse("0")
			c.update(rebooted)
			c.remove(w.pod(0, "n2"))
			w.setReady(0)
			c.update(w.objects())
			message := "0/2 nodes are available: 2 Insufficient " +
				"hugepages-2Mi. preemption: 0/2 nodes are available: 2 " +
				"No preemption victims found for incoming pod."
			c.create(w.pod(1, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 8)
		},
	}
}

// gap4aInvalidDiskCapacity: after a runtime change the kubelet reads zero
// capacity for the image file system and warns every few minutes; pods
// run.
func gap4aInvalidDiskCapacity() scenario {
	return scenario{
		expect: g2Exp("node-invalid-disk-capacity",
			"A kubelet reports InvalidDiskCapacity for its image file "+
				"system, so image garbage collection and disk eviction "+
				"cannot work; pods still run.",
			"node//n1", "digest", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			n1, _ := gapNodeFleet(c)
			for m := int32(1); m <= 4; m++ {
				c.after(5 * time.Minute)
				g2Event(c, n1, "Node", "InvalidDiskCapacity", "invalid "+
					"capacity 0 on image filesystem", "kubelet", m)
			}
		},
	}
}

// gap4aAllocatableEnforcement: the kubelet cannot enforce the reserved
// cgroup limits it was configured with and says so at every sync.
func gap4aAllocatableEnforcement() scenario {
	return scenario{
		expect: g2Exp("node-allocatable-enforcement-failed",
			"A kubelet warns FailedNodeAllocatableEnforcement because "+
				"its reserved cgroup cannot be set; pods run.",
			"node//n1", "digest", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			n1, _ := gapNodeFleet(c)
			for m := int32(1); m <= 6; m++ {
				c.after(10 * time.Minute)
				g2Event(c, n1, "Node", "FailedNodeAllocatableEnforcement",
					"Failed to update Node Allocatable Limits \"\": "+
						"failed to set supported cgroup subsystems for "+
						"cgroup [kubepods]: failed to find subsystem mount "+
						"for required subsystem: pids", "kubelet", m)
			}
		},
	}
}
