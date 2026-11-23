package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap10bScenarios: kinds kwatch cannot read, and what a watch gap, a late
// event or a stale cache leaves behind.
func gap10bScenarios() []scenario {
	return []scenario{
		gap10RBACNoNodes(), gap10RBACNoConfigMaps(), gap10RBACNoLogs(),
		gap10GapRecovered(), gap10GapRollout(), gap10GapNodeFlap(),
		gap10LateScheduling(), gap10DroppedKills(), gap10CountJump(),
		gap10StaleCache(),
	}
}

// g10Restarted is a pod that restarted and runs again: ready, with the
// last exit still on record.
func g10Restarted(restarts, exit int32, ago time.Duration) podState {
	return func(c *cluster, pod *corev1.Pod) {
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{ExitCode: exit,
				Reason:     "Error",
				FinishedAt: metav1.NewTime(c.now.Add(-ago))}}
	}
}

// gap10RBACNoNodes: nodes are Forbidden; three pods on worker-3 show
// NodeLost. The common node is named, not confirmed.
func gap10RBACNoNodes() scenario {
	return scenario{
		expect: expectation{Name: "rbac-hides-nodes-pods-unknown-phase",
			Description: "kwatch cannot list nodes; pods of a " +
				"StatefulSet and a Deployment on worker-3 show NodeLost. " +
				"worker-3 is the common node but unconfirmed.",
			Root: rootUnknown, Tier: "notify", MaxMessages: 1,
			Hidden: []string{"node"}, Tail: duration(10 * time.Minute)},
		build: func(c *cluster) {
			sts := g10Sts(c, "data", "pg", []string{"worker-3"}, nil)
			w := c.deployment("shop", "api", "registry.example.com/api:1", 2)
			c.list(w.objects())
			c.list(w.pod(0, "worker-1"), w.pod(1, "worker-3"))
			c.after(5 * time.Minute)
			c.update(operatorBroker(c, sts, 0, "worker-3", g10Lost),
				w.pod(1, "worker-3", g10Lost))
			g10Settle(c, w, 1)
		},
	}
}

// gap10RBACNoConfigMaps: ConfigMaps are Forbidden; the event text names
// the missing one.
func gap10RBACNoConfigMaps() scenario {
	return scenario{
		expect: expectation{Name: "rbac-hides-configmaps-event-names-it",
			Description: "kwatch cannot read ConfigMaps; pods show " +
				"CreateContainerConfigError and the event says configmap " +
				"\"api-flags\" is not found.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 1,
			Hidden: []string{"configmap"}, Tail: duration(10 * time.Minute)},
		build: func(c *cluster) {
			w := g10Fleet(c, "shop", "api", 2, gap10Nodes,
				func(spec *corev1.PodSpec) {
					spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
						ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: c.n("api-flags")}}}}
				})
			c.after(3 * time.Minute)
			message := "configmap \"api-flags\" not found"
			for i := range 2 {
				pod := w.pod(i, gap10Nodes[i], startedNow, waiting(
					"CreateContainerConfigError", message))
				c.update(pod)
				g10Event(c, pod, "Failed", "Error: "+message, "kubelet", 4,
					c.now)
			}
			g10Settle(c, w, 0)
		},
	}
}

// gap10RBACNoLogs: pod logs are Forbidden; redis is down in the same
// namespace but the link is unprovable.
func gap10RBACNoLogs() scenario {
	e := g2Exp("rbac-hides-pod-logs-dependency-refused",
		"An API exits within two seconds, eight restarts; the reason is "+
			"in logs kwatch may not read. Redis in the namespace is down "+
			"too: its own incident, unlinked by timing alone.",
		rootUnknown, "notify").budget(2).tail(10 * time.Minute)
	e.OtherRoots = []string{"deployment/shop/redis"}
	return scenario{expect: e, build: func(c *cluster) {
		api := g10Fleet(c, "shop", "api", 2, gap10Nodes, nil)
		redis := c.deployment("shop", "redis",
			"registry.example.com/redis:7", 1)
		c.list(redis.objects())
		c.list(redis.pod(0, "n3"))
		c.after(5 * time.Minute)
		for i := range 2 {
			c.update(api.pod(i, gap10Nodes[i], crashLoop(1, "Error", "", 8)))
		}
		g10Settle(c, api, 0)
		c.update(redis.pod(0, "n3", crashLoop(1, "Error", "", 8)))
		g10Settle(c, redis, 0)
	}}
}

// gap10GapRecovered: a relist shows two past restarts and a healthy pod.
func gap10GapRecovered() scenario {
	return scenario{
		expect: g2Quiet("watch-gap-crash-and-recover-quiet",
			"The watch drops for five minutes; a pod crashed twice and "+
				"recovered in the gap. After the relist it is Ready with "+
				"restartCount 2.").tail(10 * time.Minute),
		build: func(c *cluster) {
			for _, node := range gap10Nodes {
				c.list(c.node(node, "zone-a"))
			}
			w := c.deployment("shop", "api", "registry.example.com/api:1", 3)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2", g10Restarted(2, 1,
				2*time.Minute)), w.pod(2, "n3"))
		},
	}
}

// gap10GapRollout: after a six-minute gap the rollout is already
// crash-looping on every replica.
func gap10GapRollout() scenario {
	return scenario{
		expect: expectation{Name: "watch-gap-hides-rollout-start",
			Description: "kwatch relists after a six-minute gap and finds " +
				"revision 12 with every new pod in CrashLoopBackOff and " +
				"the old ReplicaSet at zero; the edit itself was not " +
				"seen.", Root: "deployment/shop/api", Tier: "notify",
			MaxMessages: 2, SyncAfter: duration(30 * time.Second),
			Tail: duration(10 * time.Minute)},
		build: func(c *cluster) {
			for _, node := range gap10Nodes {
				c.list(c.node(node, "zone-a"))
			}
			w := c.deployment("shop", "api", "registry.example.com/api:11", 3)
			old := w.replicaSet
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/api:12"
			})
			zero := int32(0)
			old.Spec.Replicas, old.Status = &zero, appsv1.ReplicaSetStatus{}
			since := c.now.Add(-6 * time.Minute)
			rs.CreationTimestamp = metav1.NewTime(since)
			w.setReady(0)
			c.list(w.deployment, old, rs)
			for i, node := range gap10Nodes {
				c.list(w.pod(i, node, createdAt(since), crashLoop(2, "Error",
					"fatal: unknown flag --legacy-mode", 6)))
			}
		},
	}
}

// gap10GapNodeFlap: a node was NotReady for 90 seconds inside the gap and
// is back.
func gap10GapNodeFlap() scenario {
	return scenario{
		expect: g2Quiet("watch-gap-node-flaps-quiet",
			"During a four-minute watch gap a node goes NotReady for 90 "+
				"seconds and returns; after the relist it is Ready and "+
				"its pods are Ready again.").tail(10 * time.Minute),
		build: func(c *cluster) {
			node := c.node("worker-2", "zone-a")
			setNodeCondition(node, corev1.NodeReady, corev1.ConditionTrue,
				"KubeletReady", "kubelet is posting ready status",
				g10Ago(c, 2*time.Minute))
			c.list(c.node("worker-1", "zone-a"), node)
			w := c.deployment("shop", "api", "registry.example.com/api:1", 2)
			c.list(w.objects())
			c.list(w.pod(0, "worker-1"), w.pod(1, "worker-2"))
		},
	}
}

// gap10LateScheduling: the pod's condition already says why; the event
// arrives four minutes late.
func gap10LateScheduling() scenario {
	return scenario{
		expect: g2Exp("late-unschedulable-event-after-pending-seen",
			"A pod is Pending four minutes on an untolerated taint, "+
				"visible in its condition; the FailedScheduling event "+
				"arrives four minutes late.",
			"scheduling//had untolerated taint", "notify").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			for _, node := range []string{"n1", "n2", "n3", "n4", "n5"} {
				c.list(c.node(node, "zone-a"))
			}
			w := c.deployment("jobs", "batch", "registry.example.com/b:1", 1)
			w.setReady(0)
			c.list(w.objects())
			message := "0/5 nodes are available: 5 node(s) had untolerated " +
				"taint {dedicated: gpu}. preemption: 0/5 nodes are " +
				"available: 5 Preemption is not helpful for scheduling."
			pod := w.pod(0, "", pendingUnscheduled(message))
			c.create(pod)
			for range 4 {
				c.after(time.Minute)
				c.update(last(c, pod))
			}
			c.warn(c.warningEvent(last(c, pod), "Pod", "FailedScheduling",
				message, "default-scheduler", 1))
		},
	}
}

// gap10DroppedKills: liveness kills every 90 seconds; seven in eight
// events are dropped and only BackOff survives.
func gap10DroppedKills() scenario {
	return scenario{
		expect: g2Exp("dropped-kill-events-liveness-kills",
			"A container is killed by its liveness probe every 90 "+
				"seconds (exit 137, 20 restarts); Killing and Unhealthy "+
				"events are dropped, a BackOff survives now and then.",
			"deployment/shop/api", "notify", "node//n1").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			w := g10Fleet(c, "shop", "api", 2, gap10Nodes,
				func(spec *corev1.PodSpec) {
					spec.Containers[0].LivenessProbe = &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.
							HTTPGetAction{Path: "/healthz"}}}
				})
			for round := int32(1); round <= 6; round++ {
				c.after(90 * time.Second)
				pod := w.pod(0, "n1", crashLoop(137, "Error", "", 14+round))
				c.update(pod)
				if round%3 == 0 {
					g10Event(c, pod, "BackOff", "Back-off restarting "+
						"failed container app", "kubelet", round, c.now)
				}
			}
			g10Settle(c, w, 1)
		},
	}
}

// gap10CountJump: one aggregated event whose count rises while only its
// lastTimestamp moves.
func gap10CountJump() scenario {
	return scenario{
		expect: g2Exp("aggregated-event-count-jump",
			"One aggregated readiness event on a pod: first seen 08:00, "+
				"count 600 at 08:50. kwatch sees one add and updates, not "+
				"600 events.", "deployment/shop/web", "notify").
			budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			w := g10Fleet(c, "shop", "web", 3, gap10Nodes, nil)
			first := c.now
			bad := w.pod(2, "n3", notReady)
			c.update(bad)
			for _, count := range []int32{1, 100, 250, 400, 600} {
				ev := c.warningEvent(bad, "Pod", "Unhealthy", "Readiness "+
					"probe failed: HTTP probe failed with statuscode: 503",
					"kubelet", count)
				ev.FirstTimestamp = metav1.NewTime(first)
				c.warn(ev)
				c.after(10 * time.Minute)
			}
			g10Settle(c, w, 2)
		},
	}
}

// gap10StaleCache: the Deployment is deleted but the pod's DELETED
// notification is lost; the next relist removes it.
func gap10StaleCache() scenario {
	return scenario{
		expect: expectation{Name: "stale-cache-pod-already-deleted",
			Description: "A crash-looping Deployment is deleted at 14:00 " +
				"but the pod's DELETED notification is lost; the cache " +
				"keeps the pod until a relist ten minutes later.",
			// The resolve message counts toward the budget: the
			// opening alert and the resolve.
			Root: rootUnknown, Tier: "notify", MaxMessages: 2,
			Tail: duration(15 * time.Minute)},
		build: func(c *cluster) {
			w := g10Fleet(c, "shop", "api", 1, gap10Nodes, nil)
			pod := w.pod(0, "n1", crashLoop(1, "Error", "boot failed", 6))
			c.update(pod)
			g10Settle(c, w, 0)
			c.after(5 * time.Minute)
			c.remove(w.deployment, w.replicaSet)
			c.after(10 * time.Minute)
			c.remove(pod)
		},
	}
}
