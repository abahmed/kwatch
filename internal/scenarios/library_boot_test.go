package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// bootScenarios are node pools scaled up from zero. The first minutes
// are noisy by nature (pods pending, creating, not ready), so kwatch
// waits for the boot to end before it reports what is still wrong.
func bootScenarios() []scenario {
	return []scenario{poolBootQuiet(), poolBootBroken()}
}

// bootPool is the pool the boot scenarios scale up.
const bootPool = "workers"

// poolBootQuiet: the morning scale-up. A pool of three nodes boots, the
// pods wait for it, create their sandboxes (one sandbox event, as a
// fresh node's network plugin produces) and are ready within eight
// minutes. Nothing is wrong, so nothing is said.
func poolBootQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "pool-boot-quiet",
			Description: "A node pool scales from zero to three nodes; " +
				"pods are pending, creating and not ready for a few " +
				"minutes, then all become ready within eight minutes.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			web, api := bootWorkloads(c)
			bootPending(c, web, api)
			c.after(time.Minute)
			bootNodes(c)
			bootCreating(c, web, api)
			c.after(2 * time.Minute)
			c.warn(c.warningEvent(web.pod(0, "n1"), "Pod",
				"FailedCreatePodSandBox", "network plugin is not ready: "+
					"cni config uninitialized", "kubelet", 1))
			c.after(3 * time.Minute)
			bootReady(c, web, api, -1)
		},
	}
}

// poolBootBroken: the same boot, but three minutes after the pods are
// ready one replica of the API loses readiness and stays unready. The
// boot is still on, so nothing is said until it is over; then the
// replica is reported, rooted at the workload. The pool and the zone,
// which merely booted, are not blamed.
func poolBootBroken() scenario {
	return scenario{
		expect: expectation{
			Name: "pool-boot-broken",
			Description: "A node pool boots and every pod comes up; " +
				"three minutes later one API replica turns not ready " +
				"and is still not ready when the boot is over.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"nodepool//" + bootPool, "zone//zone-a",
				"node//n1"},
			Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			web, api := bootWorkloads(c)
			bootPending(c, web, api)
			c.after(time.Minute)
			bootNodes(c)
			bootCreating(c, web, api)
			c.after(4 * time.Minute)
			bootReady(c, web, api, -1)
			c.after(3 * time.Minute)
			bootLoseReadiness(c, api)
		},
	}
}

// bootWorkloads lists a web Deployment of three replicas and an API of
// two, with no pod yet: their nodes do not exist.
func bootWorkloads(c *cluster) (web, api *workload) {
	web = c.deployment("shop", "web", "registry.example.com/web:1.0", 3)
	api = c.deployment("shop", "api", "registry.example.com/api:1.0", 2)
	web.setReady(0)
	api.setReady(0)
	c.list(web.objects())
	c.list(api.objects())
	return web, api
}

// bootPending creates every pod unscheduled: the pool has no node.
func bootPending(c *cluster, web, api *workload) {
	pending := pendingUnscheduled("0/0 nodes are available")
	for i := range 3 {
		c.create(web.pod(i, "", pending))
	}
	for i := range 2 {
		c.create(api.pod(i, "", pending))
	}
}

// bootNodes adds the three nodes of the pool, created now.
func bootNodes(c *cluster) {
	for _, name := range []string{"n1", "n2", "n3"} {
		node := c.node(name, "zone-a")
		node.Labels["karpenter.sh/nodepool"] = bootPool
		node.CreationTimestamp = metav1.NewTime(c.now)
		setNodeCondition(node, corev1.NodeReady, corev1.ConditionTrue,
			"KubeletReady", "", c.now)
		c.create(node)
	}
}

// bootCreating binds the pods to the new nodes, still creating.
func bootCreating(c *cluster, web, api *workload) {
	nodes := []string{"n1", "n2", "n3"}
	for i := range 3 {
		c.update(web.pod(i, nodes[i], creating))
	}
	for i := range 2 {
		c.update(api.pod(i, nodes[i], creating))
	}
}

// bootLoseReadiness turns the second API replica not ready.
func bootLoseReadiness(c *cluster, api *workload) {
	c.update(api.pod(1, "n2", notReady))
	api.setReady(1)
	c.update(api.objects())
}

// bootReady starts every pod. A stuck replica of the API, when stuck is
// not negative, stays running but not ready.
func bootReady(c *cluster, web, api *workload, stuck int) {
	nodes := []string{"n1", "n2", "n3"}
	started := func(c *cluster, pod *corev1.Pod) {
		startedNow(c, pod)
	}
	for i := range 3 {
		c.update(web.pod(i, nodes[i], started))
	}
	for i := range 2 {
		if i == stuck {
			c.update(api.pod(i, nodes[i], started, notReady))
			continue
		}
		c.update(api.pod(i, nodes[i], started))
	}
	web.setReady(3)
	api.setReady(2)
	if stuck >= 0 {
		api.setReady(1)
	}
	c.update(web.objects())
	c.update(api.objects())
}

// creating is a pod bound to a node whose containers are being created.
func creating(c *cluster, pod *corev1.Pod) {
	now := metav1.NewTime(c.now)
	pod.Status = corev1.PodStatus{
		Phase: corev1.PodPending, StartTime: &now,
		QOSClass: corev1.PodQOSBurstable,
		Conditions: []corev1.PodCondition{
			{Type: corev1.PodScheduled, Status: corev1.ConditionTrue,
				LastTransitionTime: now},
			{Type: corev1.PodReady, Status: corev1.ConditionFalse,
				Reason: "ContainersNotReady", LastTransitionTime: now},
		},
	}
	for _, container := range pod.Spec.Containers {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses,
			corev1.ContainerStatus{
				Name: container.Name, Image: container.Image,
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "ContainerCreating"},
				},
			})
	}
}
