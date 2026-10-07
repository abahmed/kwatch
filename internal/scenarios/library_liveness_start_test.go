package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// livenessStartScenarios are liveness probes that kill containers
// while they start, with and without a history of how long starts take.
func livenessStartScenarios() []scenario {
	return []scenario{livenessSlowStart(), livenessNeverReady(),
		livenessKilledAfterStart()}
}

// livenessSlowStart: the api scales out. Its five ready replicas each
// needed about 75 seconds to become ready, but liveness gives 40 (a
// 10s delay and three 10s periods), so every new replica is killed
// before it is up. The Deployment is the root.
func livenessSlowStart() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-slow-start",
			Description: "New replicas are killed by a liveness probe " +
				"that allows 40s while the last five starts took 75s.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := slowStartWorkload(c, 5)
			c.list(w.pod(0, "n1", readyAfter(72*time.Second)),
				w.pod(1, "n2", readyAfter(74*time.Second)),
				w.pod(2, "n1", readyAfter(75*time.Second)),
				w.pod(3, "n2", readyAfter(76*time.Second)),
				w.pod(4, "n1", readyAfter(78*time.Second)))
			c.after(time.Minute)
			setReplicas(w, 7)
			w.setReady(5)
			c.update(w.objects())
			killedBeforeReady(c, w, []int{5, 6})
		},
	}
}

// livenessNeverReady: a new revision whose pods are all killed by
// liveness before they are ready. No pod ever became ready, so kwatch
// has no history of how long a start takes and says only what it saw.
func livenessNeverReady() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-never-ready",
			Description: "Every pod of a new Deployment is killed by " +
				"its liveness probe before it is ready; no start has " +
				"ever completed.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := slowStartWorkload(c, 2)
			c.after(time.Minute)
			killedBeforeReady(c, w, []int{0, 1})
		},
	}
}

// livenessKilledAfterStart: the same workload and history, but each
// pod ran for two minutes (well past a start) before the kill. The
// start was not cut short, so the budget is not blamed.
func livenessKilledAfterStart() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-kill-after-start",
			Description: "Pods that started in 75s are killed by " +
				"liveness after running for two minutes.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := slowStartWorkload(c, 4)
			c.list(w.pod(0, "n1", readyAfter(74*time.Second)),
				w.pod(1, "n2", readyAfter(75*time.Second)),
				w.pod(2, "n1", readyAfter(76*time.Second)),
				w.pod(3, "n2", readyAfter(75*time.Second)))
			c.after(time.Minute)
			killedLate(c, w, []int{0, 1})
		},
	}
}

// slowStartWorkload is shop/api with a readiness probe and a liveness
// probe that allows 40s: a 10s delay and three failures 10s apart.
func slowStartWorkload(c *cluster, replicas int32) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:5",
		replicas)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Ports = []corev1.ContainerPort{{
			Name: "http", ContainerPort: 8080}}
		spec.Containers[0].ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/ready", Port: intstr.FromInt32(8080)}},
		}
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz", Port: intstr.FromInt32(8080)}},
			InitialDelaySeconds: 10, PeriodSeconds: 10, FailureThreshold: 3,
		}
	})
	c.list(w.objects())
	return w
}

// killedBeforeReady restarts the listed pods five rounds in a row, each
// kill 42 seconds into a run: liveness gave up before the start ended.
func killedBeforeReady(c *cluster, w *workload, pods []int) {
	killRounds(c, w, pods, 42*time.Second)
}

// killedLate is the same loop with runs of two minutes.
func killedLate(c *cluster, w *workload, pods []int) {
	killRounds(c, w, pods, 2*time.Minute)
}

func killRounds(c *cluster, w *workload, pods []int, run time.Duration) {
	probe := "Liveness probe failed: Get \"http://10.244.0.12:8080/" +
		"healthz\": dial tcp 10.244.0.12:8080: connect: connection refused"
	nodes := []string{"n1", "n2"}
	for restarts := int32(3); restarts <= 7; restarts++ {
		for _, i := range pods {
			pod := w.pod(i, nodes[i%2], liveKilled(restarts, run))
			ev := c.warningEvent(pod, "Pod", "Unhealthy", probe,
				"kubelet", restarts*3)
			ev.InvolvedObject.FieldPath = "spec.containers{app}"
			c.warn(ev)
			c.update(pod)
		}
		c.after(run + 3*time.Second)
	}
}

// liveKilled is a container that is running again, not ready, after a
// run of the given length that ended in SIGTERM (exit 143) a moment ago.
func liveKilled(restarts int32, run time.Duration) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.State = corev1.ContainerState{Running: &corev1.
			ContainerStateRunning{StartedAt: metav1.NewTime(
			c.now.Add(-2 * time.Second))}}
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 143, Reason: "Error",
				StartedAt:  metav1.NewTime(c.now.Add(-run - 3*time.Second)),
				FinishedAt: metav1.NewTime(c.now.Add(-3 * time.Second)),
			},
		}
	}
}
