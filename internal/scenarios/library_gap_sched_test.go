package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gapSchedScenarios are releases and edits whose pods never get going
// because of what the new pod spec asks for.
func gapSchedScenarios() []scenario {
	return []scenario{
		gapNodeSelectorNoMatch(), gapAntiAffinityPending(),
		gapCommandNotFound(), gapReadOnlyRootFS(), gapHPANoCPURequest(),
	}
}

// gapRelease lists a healthy two-replica shop Deployment on two nodes and
// rolls it out with mutate. The new ReplicaSet has no pods yet and the
// old pods keep serving, as in a rolling update that surges by one.
func gapRelease(
	c *cluster, name string, mutate func(*corev1.PodSpec),
) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", name, "registry.example.com/"+name+":2.5", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(2 * time.Minute)
	rs := w.rollout(mutate)
	d := w.deployment
	d.Status.Replicas, d.Status.UpdatedReplicas = 3, 1
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 2, 2
	d.Status.UnavailableReplicas = 1
	c.update(d)
	c.create(rs)
	return w
}

// gapPending is a pod that is Pending, whatever else state says.
func gapPending(state podState) podState {
	return func(c *cluster, pod *corev1.Pod) {
		state(c, pod)
		pod.Status.Phase = corev1.PodPending
	}
}

// gapKubelet delivers a kubelet Warning event about pod.
func gapKubelet(
	c *cluster, pod *corev1.Pod, reason, message string, count int32,
) {
	event := c.warningEvent(pod, "Pod", reason, message, "kubelet", count)
	event.InvolvedObject.UID = pod.UID
	c.warn(event)
}

// gapRounds updates the new ReplicaSet's first pod, on n1, to state once
// per step; after each update warn may deliver the kubelet's events.
func gapRounds(
	c *cluster, w *workload, rounds int32, step time.Duration,
	state func(round int32) podState, warn func(*corev1.Pod, int32),
) {
	created := c.now
	for round := int32(1); round <= rounds; round++ {
		pod := w.pod(0, "n1", createdAt(created), state(round))
		c.update(pod)
		if warn != nil {
			warn(pod, round)
		}
		c.after(step)
	}
}

// gapNodeSelectorNoMatch: a Deployment is edited to select nodes of pool
// gpu-b after a node pool was renamed; no node carries the label, so the
// new pod stays Pending while the old pods serve. The edit is the cause.
func gapNodeSelectorNoMatch() scenario {
	return scenario{
		expect: expectation{
			Name: "sched-node-selector-no-match",
			Description: "A Deployment edit adds a nodeSelector that no " +
				"node matches; the new pod is Pending on \"didn't match " +
				"Pod's node affinity/selector\" while the old pods run.",
			Root: "scheduling//didn't match Pod's node affinity/selector",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.NodeSelector = map[string]string{"pool": "gpu-b"}
			})
			message := "0/2 nodes are available: 2 node(s) didn't match " +
				"Pod's node affinity/selector. preemption: 0/2 nodes " +
				"are available: 2 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 8)
		},
	}
}

// gapAntiAffinityPending: a Deployment with required per-node
// anti-affinity is scaled from three to four replicas on a three-node
// cluster; the fourth pod can never be placed. The spread rule is the
// cause, not a node.
func gapAntiAffinityPending() scenario {
	return scenario{
		expect: expectation{
			Name: "sched-anti-affinity-pending",
			Description: "A Deployment with required pod anti-affinity " +
				"per node is scaled to four replicas on three nodes; " +
				"the fourth stays Pending on \"didn't match pod " +
				"anti-affinity rules\".",
			Root: "scheduling//didn't match pod anti-affinity rules",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2", "node//n3",
				"scheduling//Insufficient cpu"},
		},
		build: func(c *cluster) {
			for _, name := range []string{"n1", "n2", "n3"} {
				c.list(c.node(name, "zone-a"))
			}
			w := c.deployment("shop", "search",
				"registry.example.com/search:6", 3)
			configTemplate(w, func(spec *corev1.PodSpec) {
				term := corev1.PodAffinityTerm{
					TopologyKey: corev1.LabelHostname,
				}
				spec.Affinity = &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.
							PodAffinityTerm{term},
					},
				}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n3"))
			c.after(5 * time.Minute)
			*w.deployment.Spec.Replicas = 4
			w.setReady(3)
			c.update(w.objects())
			message := "0/3 nodes are available: 3 node(s) didn't match " +
				"pod anti-affinity rules. preemption: 0/3 nodes are " +
				"available: 3 No preemption victims found for incoming " +
				"pod."
			c.create(w.pod(3, "", pendingUnscheduled(message)))
			for m := int32(1); m <= 8; m++ {
				c.after(time.Minute)
				pod := last(c, w.pod(3, ""))
				c.warn(c.warningEvent(pod, "Pod", "FailedScheduling",
					message, "default-scheduler", m))
			}
		},
	}
}

// gapCommandNotFound: a release sets command /app/serve that the image
// does not contain; the container cannot be started and the kubelet
// reports "no such file or directory".
func gapCommandNotFound() scenario {
	return scenario{
		expect: expectation{
			Name: "release-command-not-found",
			Description: "A release sets a container command that the " +
				"image lacks; the new pod fails with RunContainerError " +
				"\"no such file or directory\".",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.Containers[0].Command = []string{"/app/serve"}
			})
			message := "failed to create containerd task: failed to " +
				"create shim task: OCI runtime create failed: runc " +
				"create failed: unable to start container process: " +
				"exec: \"/app/serve\": stat /app/serve: no such file " +
				"or directory: unknown"
			gapRounds(c, w, 6, 45*time.Second, func(int32) podState {
				return gapPending(waiting("RunContainerError", message))
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "Failed", "Error: "+message, round)
			})
		},
	}
}

// gapReadOnlyRootFS: a hardening release sets readOnlyRootFilesystem and
// the application, which writes its pid file under /var/run, exits at
// every start.
func gapReadOnlyRootFS() scenario {
	return scenario{
		expect: expectation{
			Name: "release-read-only-root-filesystem",
			Description: "A release turns on readOnlyRootFilesystem; the " +
				"new pods crash-loop because the app cannot write its " +
				"pid file.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.Containers[0].SecurityContext = &corev1.
					SecurityContext{ReadOnlyRootFilesystem: boolPtr(true)}
			})
			message := "FATAL: could not create lock file " +
				"\"/var/run/api/api.pid\": Read-only file system"
			gapRounds(c, w, 6, 45*time.Second, func(round int32) podState {
				return crashLoop(1, "Error", message, round)
			}, nil)
		},
	}
}

// gapHPANoCPURequest: an HPA scales on CPU utilisation but the pods of
// its target set no CPU request, so the controller cannot compute a
// percentage. The metrics API is healthy; the target is the cause.
func gapHPANoCPURequest() scenario {
	return scenario{
		expect: expectation{
			Name: "hpa-target-no-cpu-request",
			Description: "An HPA on CPU utilisation reports \"missing " +
				"request for cpu\" because its target Deployment's pods " +
				"set no CPU request; the metrics API is healthy.",
			Root: "horizontalpodautoscaler/shop/web", Tier: "digest",
			MaxMessages: 2,
			MustNotBlame: []string{"apiservice//v1beta1.metrics.k8s.io",
				"deployment/kube-system/metrics-server"},
			Tail: duration(25 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			server := metricsServer(c)
			c.list(server.objects())
			c.list(server.pod(0, "n1"), server.pod(1, "n1"))
			c.list(clusterService(c, "kube-system", "metrics-server", 10250))
			c.list(metricsSlice(c, server, true))
			c.list(clusterAPIService(c, "True", "Passed",
				"all checks passed"))
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.list(clusterHPA(c, "shop", "web", "True",
				"ValidMetricFound", ""))
			c.after(time.Minute)
			message := "the HPA was unable to compute the replica " +
				"count: failed to get cpu utilization: missing request " +
				"for cpu in container app of Pod " + w.pod(0, "n1").Name
			for m := int32(1); m <= 6; m++ {
				hpa := clusterHPA(c, "shop", "web", "False",
					"FailedGetResourceMetric", message)
				c.update(hpa)
				c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
					"FailedGetResourceMetric", message,
					"horizontal-pod-autoscaler", m))
				c.after(time.Minute)
			}
		},
	}
}
