package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var gap10Nodes = []string{"n1", "n2", "n3"}

// gap10aScenarios: events dropped, expired, replayed or out of order, and
// kwatch restarting under a live incident.
func gap10aScenarios() []scenario {
	return []scenario{
		gap10DroppedOOM(), gap10EventTTL(), gap10ResyncReplay(),
		gap10RestartMidIncident(), gap10RestartNodeLoss(),
		gap10SkewBehind(), gap10SkewAhead(), gap10EventBeforePod(),
		gap10EventBeforeOwner(), gap10RBACNoEvents(),
	}
}

// gap10DroppedOOM: the event rate limiter dropped every event; pod status
// alone names the OOM kill.
func gap10DroppedOOM() scenario {
	return scenario{
		expect: g2Exp("dropped-oom-event-crash-loop",
			"Five OOM kills in twelve minutes with every Killing and "+
				"BackOff event dropped by the event rate limiter; only "+
				"pod status and restart counts remain.",
			"deployment/shop/api", "notify", "node//n1").budget(2).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			w := g10Fleet(c, "shop", "api", 3, gap10Nodes,
				func(spec *corev1.PodSpec) {
					spec.Containers[0].Resources.Limits = corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi")}
				})
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.after(150 * time.Second)
				c.update(w.pod(0, "n1", crashLoop(137, "OOMKilled", "",
					restarts)))
				g10Settle(c, w, 2)
			}
		},
	}
}

// gap10EventTTL: kwatch starts 75 minutes into an image typo, after the
// event TTL erased every event; the waiting state remains.
func gap10EventTTL() scenario {
	return scenario{
		expect: g2Exp("event-ttl-expired-before-kwatch-start",
			"A typo in an image tag, 75 minutes old; kwatch starts after "+
				"the one-hour event TTL erased the events and sees only "+
				"ImagePullBackOff and the image string.",
			"deployment/billing/worker", "notify", "node//n1").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			since := c.now.Add(-75 * time.Minute)
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			ledger := c.deployment("billing", "ledger",
				"registry.example/ledger:3.0", 2)
			c.list(ledger.objects())
			c.list(ledger.pod(0, "n1"), ledger.pod(1, "n2"))
			w := c.deployment("billing", "worker",
				"registry.example/worker:v2.0", 2)
			image := "registry.example/worker:v2.1O"
			old := w.replicaSet
			old.Spec.Replicas, old.Status = new(int32), appsv1.ReplicaSetStatus{}
			w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = image
			})
			w.setReady(0)
			w.replicaSet.CreationTimestamp = metav1.NewTime(since)
			c.list(w.objects())
			c.list(old)
			message := "failed to pull and unpack image \"" + image +
				"\": failed to resolve reference \"" + image +
				"\": not found"
			for i := range 2 {
				c.list(w.pod(i, "n"+itoa(i+1), createdAt(since),
					waiting("ErrImagePull", message),
					notReadySince(since)))
			}
		},
	}
}

// gap10ResyncReplay: a resync re-delivers old Warning events of a crash
// that ended 40 minutes ago.
func gap10ResyncReplay() scenario {
	return scenario{
		expect: g2Quiet("resync-replays-resolved-warnings",
			"A crash loop ended 40 minutes ago; a resync re-delivers its "+
				"old BackOff and Unhealthy events with unchanged "+
				"timestamps. Nothing is wrong.").tail(10 * time.Minute),
		build: func(c *cluster) {
			w := g10Fleet(c, "shop", "api", 3, gap10Nodes, nil)
			c.after(30 * time.Minute)
			old := g10Ago(c, 40*time.Minute)
			for i := range 3 {
				pod := last(c, w.pod(i, gap10Nodes[i]))
				g10Event(c, pod, "BackOff", "Back-off restarting failed "+
					"container app", "kubelet", 12, old)
				g10Event(c, pod, "Unhealthy", "Readiness probe failed: "+
					"HTTP probe failed with statuscode: 503", "kubelet", 7,
					old)
			}
		},
	}
}

// gap10RestartMidIncident: kwatch relists a bad rollout nine minutes in.
// The page the first session sent cannot be recorded in one log; this is
// the second session's view.
func gap10RestartMidIncident() scenario {
	return scenario{
		expect: expectation{Name: "kwatch-restart-mid-incident-no-second-page",
			Description: "Revision 8 of a Deployment crash-loops on a " +
				"missing config key; kwatch restarts nine minutes in and " +
				"relists everything. One page for the incident.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			SyncAfter: duration(30 * time.Second),
			Tail:      duration(10 * time.Minute)},
		build: func(c *cluster) {
			since := c.now.Add(-9 * time.Minute)
			for _, node := range gap10Nodes {
				c.list(c.node(node, "zone-a"))
			}
			w := c.deployment("shop", "api", "registry.example.com/api:8", 3)
			w.setReady(0)
			c.list(w.objects())
			for i, node := range gap10Nodes {
				c.list(w.pod(i, node, createdAt(since), crashLoop(1,
					"Error", "config key DATABASE_URL is missing", 6)))
			}
		},
	}
}

// gap10RestartNodeLoss: kwatch relists after the node already lost its
// heartbeat; the transition event was never seen.
func gap10RestartNodeLoss() scenario {
	return scenario{
		expect: expectation{Name: "kwatch-restart-during-node-loss",
			Description: "A node goes NotReady and kwatch restarts three " +
				"minutes later; after the relist the node already holds " +
				"the unreachable taint and Unknown condition.",
			Root: "node//worker-3", Tier: "page", MaxMessages: 2,
			SyncAfter:    duration(30 * time.Second),
			Tail:         duration(10 * time.Minute),
			MustNotBlame: []string{"statefulset/data/cache"}},
		build: func(c *cluster) {
			nodes := []string{"worker-1", "worker-2", "worker-3"}
			lost := c.node("worker-3", "zone-a")
			setNodeCondition(lost, corev1.NodeReady, corev1.ConditionUnknown,
				"NodeStatusUnknown", "Kubelet stopped posting node status.",
				g10Ago(c, 3*time.Minute))
			lost.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/unreachable", Effect: "NoExecute"}}
			c.list(c.node("worker-1", "zone-a"), c.node("worker-2",
				"zone-a"), lost)
			w := c.deployment("shop", "api", "registry.example.com/api:1", 3)
			w.setReady(2)
			c.list(w.objects())
			c.list(w.pod(0, nodes[0]), w.pod(1, nodes[1]),
				w.pod(2, nodes[2], g10Lost))
			sts := g10Sts(c, "data", "cache", []string{"worker-3"}, nil)
			c.update(operatorBroker(c, sts, 0, "worker-3", g10Lost))
		},
	}
}

// gap10SkewBehind: the failing pods' node clock is 90 seconds behind, so
// their events predate the ConfigMap edit that broke them.
func gap10SkewBehind() scenario {
	return scenario{
		expect: g2Exp("clock-skew-node-events-predate-the-edit",
			"A ConfigMap edit breaks new pods on a node whose clock is "+
				"90 seconds behind; their events are stamped before the "+
				"edit.",
			"configmap/shop/api-config", "notify").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			cm := configMap(c, "shop", "api-config",
				map[string]string{"flags": "a", "mode": "x"})
			w := g10Fleet(c, "shop", "api", 2, []string{"n1", "worker-5"},
				func(spec *corev1.PodSpec) {
					spec.Containers[0].Env = []corev1.EnvVar{{Name: "MODE",
						ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.
							ConfigMapKeySelector{LocalObjectReference: corev1.
							LocalObjectReference{Name: cm.Name},
							Key: "mode"}}}}
				})
			c.list(cm)
			c.after(5 * time.Minute)
			edited := last(c, cm)
			delete(edited.Data, "mode")
			c.update(edited)
			editAt := c.now
			c.after(40 * time.Second)
			pod := w.pod(1, "worker-5", startedNow, waiting(
				"CreateContainerConfigError", "couldn't find key mode in "+
					"ConfigMap shop/api-config"))
			c.update(pod)
			g10Event(c, pod, "Failed", "Error: couldn't find key mode in "+
				"ConfigMap shop/api-config", "kubelet", 3,
				editAt.Add(-70*time.Second))
			g10Settle(c, w, 1)
		},
	}
}

// gap10SkewAhead: the node clock is six minutes fast; the FailedMount
// event is stamped in the future.
func gap10SkewAhead() scenario {
	return scenario{
		expect: g2Exp("clock-skew-future-events-secret-missing",
			"A pod's FailedMount event is stamped six minutes in the "+
				"future by a fast node clock; the Secret does not exist.",
			"secret/media/encoder-creds", "notify").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			sts := g10Sts(c, "media", "encoder", []string{"n1"},
				g10Secret(c.n("encoder-creds")))
			c.after(2 * time.Minute)
			pod := operatorBroker(c, sts, 0, "n1", gapPending(waiting(
				"ContainerCreating", "")), notReady)
			c.update(pod)
			g10Event(c, pod, "FailedMount", "MountVolume.SetUp failed for "+
				"volume \"creds\" : secret \"encoder-creds\" not found",
				"kubelet", 2, c.now.Add(6*time.Minute))
		},
	}
}

// gap10EventBeforePod: the FailedMount event reaches kwatch before the
// Pod and its ReplicaSet.
func gap10EventBeforePod() scenario {
	return scenario{
		expect: g2Exp("event-before-pod-object-missing-secret",
			"A new Deployment's pod fails to mount a missing Secret; its "+
				"event arrives 400 ms before the Pod and before the "+
				"ReplicaSet.", "secret/shop/checkout-creds", "notify").
			budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "checkout",
				"registry.example.com/checkout:1", 1)
			configTemplate(w, g10Secret(c.n("checkout-creds")))
			w.setReady(0)
			c.create(w.deployment)
			pod := w.pod(0, "n1", startedNow, gapPending(waiting(
				"ContainerCreating", "")), notReady)
			g10Event(c, pod, "FailedMount", "MountVolume.SetUp failed for "+
				"volume \"creds\" : secret \"checkout-creds\" not found",
				"kubelet", 1, c.now)
			c.after(400 * time.Millisecond)
			c.create(w.replicaSet, pod)
			c.after(90 * time.Second)
			g10Event(c, pod, "FailedMount", "MountVolume.SetUp failed for "+
				"volume \"creds\" : secret \"checkout-creds\" not found",
				"kubelet", 2, c.now)
		},
	}
}

// gap10EventBeforeOwner: the pod and its pull failure arrive twenty
// seconds before its Deployment and ReplicaSet.
func gap10EventBeforeOwner() scenario {
	return scenario{
		expect: g2Exp("event-before-owner-replicaset-cached",
			"A pod hits ErrImagePull; the Deployments watch lags by "+
				"twenty seconds, so its owners arrive after the pod and "+
				"its events.", "deployment/reports/gen", "notify").
			budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("reports", "gen",
				"registry.example.com/gen:missing", 1)
			w.setReady(0)
			message := "rpc error: code = NotFound desc = failed to pull " +
				"image \"registry.example.com/gen:missing\""
			pod := w.pod(0, "n1", startedNow, waiting("ErrImagePull",
				message), notReady)
			c.create(pod)
			g10Event(c, pod, "Failed", "Failed to pull image: "+message,
				"kubelet", 1, c.now)
			c.after(20 * time.Second)
			c.create(w.deployment, w.replicaSet)
			c.after(time.Minute)
			c.update(w.pod(0, "n1", startedNow, waiting("ImagePullBackOff",
				message), notReady))
		},
	}
}

// gap10RBACNoEvents: kwatch cannot read Events; status names the pull
// failure.
func gap10RBACNoEvents() scenario {
	return scenario{
		expect: g2Exp("rbac-hides-events-status-suffices",
			"kwatch's role cannot list Events; pods show "+
				"ImagePullBackOff and the image string.",
			"deployment/shop/api", "notify").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			w := g10Fleet(c, "shop", "api", 3, gap10Nodes, nil)
			c.after(5 * time.Minute)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "ghcr.io/acme/api:v9"
			})
			w.setReady(0)
			c.update(w.deployment)
			c.create(rs)
			c.after(30 * time.Second)
			for i, node := range gap10Nodes {
				c.create(w.pod(i, node, waiting("ImagePullBackOff",
					"Back-off pulling image \"ghcr.io/acme/api:v9\"")))
			}
		},
	}
}
