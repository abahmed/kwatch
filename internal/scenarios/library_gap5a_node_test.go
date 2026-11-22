package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap5aNodeScenarios are volumes that mount slowly, runtimes that refuse
// names, nodes that stay cordoned or lack their join credentials, and
// pods that no controller replaces.
func gap5aNodeScenarios() []scenario {
	return []scenario{
		gap5aSlowFsGroupMount(), gap5aContainerNameReserved(),
		gap5aBootstrapTokenExpired(), gap5aCordonedForDays(),
		gap5aBarePodEvicted(), gap5aBarePodFailed(), gap5aDrainBarePod(),
	}
}

// gap5aPending delivers FailedScheduling for the pods idx of w once a
// minute, minutes times.
func gap5aPending(
	c *cluster, w *workload, idx []int, message string, minutes int32,
) {
	for m := int32(1); m <= minutes; m++ {
		c.after(time.Minute)
		for _, i := range idx {
			g2Event(c, last(c, w.pod(i, "")), "Pod", "FailedScheduling",
				message, "default-scheduler", m)
		}
	}
}

// gap5aBarePod is a pod no controller owns, serving app name.
func gap5aBarePod(c *cluster, namespace, name, node string) *corev1.Pod {
	meta := c.meta(namespace, name)
	meta.Labels = map[string]string{"app": meta.Name}
	pod := &corev1.Pod{ObjectMeta: meta, Spec: corev1.PodSpec{
		NodeName: c.n(node), Containers: []corev1.Container{{Name: "app",
			Image: "registry.example.com/" + name + ":1"}}}}
	running(c, pod)
	return pod
}

// gap5aSlowFsGroupMount: a StatefulSet sets fsGroup on a volume holding
// millions of files; the kubelet changes the ownership of each before it
// can start the container, and the pod waits past the mount timeout.
func gap5aSlowFsGroupMount() scenario {
	return scenario{
		expect: g2Exp("statefulset-fsgroup-recursive-mount-timeout",
			"A StatefulSet pod waits in ContainerCreating with "+
				"FailedMount \"timed out waiting for the condition\" "+
				"because fsGroup makes the kubelet change ownership of a "+
				"very large volume.",
			"statefulset/streaming/queue", "notify", "node//n1",
			"node//n2").tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			group := int64(1000)
			updated := sts.DeepCopy()
			updated.Spec.Template.Spec.SecurityContext =
				&corev1.PodSecurityContext{FSGroup: &group}
			updated.Generation, updated.Status.ObservedGeneration = 2, 2
			c.update(updated)
			c.after(10 * time.Minute)
			c.remove(operatorBroker(c, sts, 2, "n1"))
			for round := int32(1); round <= 7; round++ {
				pod := operatorBroker(c, sts, 2, "n1", startedNow,
					gapPending(waiting("ContainerCreating", "")))
				c.update(pod)
				gapKubelet(c, pod, "FailedMount", "Unable to attach or "+
					"mount volumes: unmounted volumes=[data], unattached "+
					"volumes=[data kube-api-access]: timed out waiting "+
					"for the condition", round)
				c.after(2 * time.Minute)
			}
		},
	}
}

// gap5aContainerNameReserved: the runtime of n3 refuses to create
// containers whose name it still holds for a dead one.
func gap5aContainerNameReserved() scenario {
	return scenario{
		expect: g2Exp("node-runtime-container-name-reserved",
			"New pods on one node fail with CreateContainerError "+
				"\"failed to reserve container name\" while the same "+
				"workload starts on other nodes.",
			"node//n3", "notify", "deployment/shop/api").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			*w.deployment.Spec.Replicas = 3
			w.setReady(2)
			c.update(w.objects())
			message := "failed to reserve container name \"app_api-0_shop_" +
				"6a1c_0\": name \"app_api-0_shop_6a1c_0\" is reserved " +
				"for \"4f1c0e2a\""
			created := c.now
			for round := int32(1); round <= 6; round++ {
				pod := w.pod(2, "n3", createdAt(created),
					gapPending(waiting("CreateContainerError", message)))
				c.update(pod)
				gapKubelet(c, pod, "Failed", "Error: "+message, round)
				c.after(time.Minute)
			}
		},
	}
}

// gap5aBootstrapTokenExpired: replacement nodes cannot join because the
// bootstrap token they present has expired; the pods that need room stay
// Pending on the nodes that exist.
func gap5aBootstrapTokenExpired() scenario {
	return scenario{
		expect: g2Exp("bootstrap-token-expired-nodes-cannot-join",
			"A scale-up needs new nodes whose join token expired a day "+
				"ago; no Node object appears and the new pods stay "+
				"Pending for lack of CPU. The token expiry is a Secret "+
				"value kwatch cannot read, so only the Pending for "+
				"lack of CPU is visible.",
			"scheduling//Insufficient cpu", "notify",
			"deployment/shop/web", "node//n1", "node//n2").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			token := configSecret(c, "kube-system", "bootstrap-token-abcdef",
				"token-id", "token-secret")
			token.Type = "bootstrap.kubernetes.io/token"
			token.Data["expiration"] = []byte(c.start.Add(
				-24 * time.Hour).UTC().Format(time.RFC3339))
			c.list(token)
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			*w.deployment.Spec.Replicas = 5
			w.setReady(2)
			c.update(w.objects())
			message := "0/2 nodes are available: 2 Insufficient cpu. " +
				"preemption: 0/2 nodes are available: 2 No preemption " +
				"victims found for incoming pod."
			for i := 2; i < 5; i++ {
				c.create(w.pod(i, "", pendingUnscheduled(message)))
			}
			gap5aPending(c, w, []int{2, 3, 4}, message, 8)
		},
	}
}

// gap5aCordonedForDays: n3 was cordoned for maintenance three days ago and
// never uncordoned; a scale-up finds room only on the other two nodes.
func gap5aCordonedForDays() scenario {
	return scenario{
		expect: g2Exp("node-left-cordoned-after-maintenance",
			"A node cordoned three days ago was never uncordoned; "+
				"scaled-up pods are Pending because one node is "+
				"unschedulable and the others are full. The cordon "+
				"explains the Pending, not the scheduling reason alone.",
			"node//n3", "notify", "deployment/shop/web").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			cordoned := c.node("n3", "zone-a")
			cordoned.Spec.Unschedulable = true
			cordoned.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
				TimeAdded: &metav1.Time{Time: c.start.Add(-72 * time.Hour)}}}
			c.list(cordoned)
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			*w.deployment.Spec.Replicas = 4
			w.setReady(2)
			c.update(w.objects())
			message := "0/3 nodes are available: 1 node(s) were " +
				"unschedulable, 2 Insufficient cpu. preemption: 0/3 " +
				"nodes are available: 1 Preemption is not helpful for " +
				"scheduling, 2 No preemption victims found for " +
				"incoming pod."
			c.create(w.pod(2, "", pendingUnscheduled(message)),
				w.pod(3, "", pendingUnscheduled(message)))
			gap5aPending(c, w, []int{2, 3}, message, 8)
		},
	}
}

// gap5aBarePodEvicted: a standalone cache pod is evicted under disk
// pressure; nothing recreates it and its callers fail.
func gap5aBarePodEvicted() scenario {
	return scenario{
		expect: g2Exp("bare-pod-evicted-never-replaced",
			"A standalone pod with no controller is evicted for "+
				"ephemeral storage; nothing recreates it and the "+
				"service it backed fails.",
			"pod/shop/cache-standalone", "notify", "node//n1",
			"deployment/shop/web").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			pod := gap5aBarePod(c, "shop", "cache-standalone", "n1")
			c.list(pod, clusterService(c, "shop", "cache-standalone", 6379),
				clusterSlice(c, "shop", "cache-standalone", pod))
			web := c.deployment("shop", "web", "registry.example.com/web:3", 2)
			c.list(web.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n2"))
			c.after(10 * time.Minute)
			evicted("The node was low on resource: ephemeral-storage. "+
				"Threshold quantity: 10%, available: 4%.")(c, pod)
			c.update(pod)
			c.update(clusterSlice(c, "shop", "cache-standalone"))
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(web.pod(0, "n1", crashLoop(1, "Error", "dial tcp "+
					"10.96.30.8:6379: connect: connection refused",
					restarts)), web.pod(1, "n2", crashLoop(1, "Error",
					"dial tcp 10.96.30.8:6379: connect: connection "+
						"refused", restarts)))
				web.setReady(0)
				c.update(web.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap5aBarePodFailed: a one-off migration pod with restartPolicy Never
// exits non-zero and stays Failed; nothing retries it.
func gap5aBarePodFailed() scenario {
	return scenario{
		expect: g2Exp("bare-pod-restart-never-failed",
			"A one-off pod with restartPolicy Never exits with a "+
				"non-zero code and stays Failed; no controller retries "+
				"it.", "pod/batch/migrate-once", "notify",
			"node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			pod := gap5aBarePod(c, "batch", "migrate-once", "n1")
			pod.Spec.RestartPolicy = corev1.RestartPolicyNever
			c.create(pod)
			c.after(2 * time.Minute)
			failed := pod.DeepCopy()
			failed.Status.Phase = corev1.PodFailed
			failed.Status.ContainerStatuses[0].Ready = false
			failed.Status.ContainerStatuses[0].State = corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{ExitCode: 2,
					Reason: "Error", Message: "migration 0042 failed: " +
						"duplicate key value violates unique constraint",
					StartedAt:  metav1.NewTime(c.now.Add(-time.Minute)),
					FinishedAt: metav1.NewTime(c.now)}}
			c.update(failed)
			c.after(10 * time.Minute)
		},
	}
}

// gap5aDrainBarePod: an upgrade cordoned n1 two hours ago and waits for a
// debug pod that no controller owns.
func gap5aDrainBarePod() scenario {
	return scenario{
		expect: g2Quiet("drain-waits-on-pod-without-owner",
			"A node cordoned for an upgrade stays cordoned for hours "+
				"because a pod without a controller cannot be moved. "+
				"Nothing Kubernetes reports fails.").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			n1, fleet := gapNodeFleet(c)
			pod := gap5aBarePod(c, "ops", "debug-shell", "n1")
			c.list(pod)
			advCordon(c, n1)
			for _, w := range fleet {
				c.remove(w.pod(0, "n1"))
				c.create(w.pod(2, "n3"))
				w.setReady(2)
				c.update(w.objects())
			}
			for range 4 {
				c.after(30 * time.Minute)
				c.update(pod.DeepCopy())
			}
		},
	}
}
