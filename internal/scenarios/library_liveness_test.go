package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// livenessKillLoop: a release makes the app answer its liveness probe
// too slowly, so the kubelet kills each pod every ~40s (exit 143) and
// the restart count climbs. It is a crash loop of one Deployment, not
// a probe curiosity and not a node problem.
func livenessKillLoop() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-kill-loop",
			Description: "The kubelet kills every pod of a Deployment " +
				"every 40s for a failing liveness probe; restarts " +
				"climb and it reads as a crash loop.",
			Root: "deployment/shop/web", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := livenessWorkload(c)
			c.after(time.Minute)
			// The loop is already under way when kwatch sees it: the
			// pods have been killed twice before the first event.
			for round := int32(3); round <= 10; round++ {
				c.after(40 * time.Second)
				livenessKillRound(c, w, round)
			}
		},
	}
}

// livenessSingleKill: one liveness kill after a slow start, then the
// pod is healthy. One restart is a probe doing its job: nothing to say.
func livenessSingleKill() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-single-kill",
			Description: "One pod is killed once by its liveness " +
				"probe and then stays healthy.",
			Quiet: true,
		},
		build: func(c *cluster) {
			w := livenessWorkload(c)
			c.after(time.Minute)
			livenessKillRound(c, w, 1)
			c.after(10 * time.Minute)
		},
	}
}

// livenessWorkload is shop/web with two pods and a liveness probe.
func livenessWorkload(c *cluster) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "web", "registry.example.com/web:9", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/healthz",
					Port: intstr.FromInt32(8080)}},
			PeriodSeconds: 10, FailureThreshold: 3,
		}
	})
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	return w
}

// livenessKillRound records the kubelet's Unhealthy events and the
// restart that follows on every pod: running again, restartCount up,
// last termination SIGTERM.
func livenessKillRound(c *cluster, w *workload, restarts int32) {
	for i, node := range []string{"n1", "n2"} {
		pod := w.pod(i, node, killedByLiveness(restarts))
		ev := c.warningEvent(pod, "Pod", "Unhealthy",
			"Liveness probe failed: Get \"http://10.244.0.9:8080/healthz"+
				"\": context deadline exceeded", "kubelet", 3)
		// The kubelet names the probed container.
		ev.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(ev)
		c.update(pod)
	}
}

// killedByLiveness is a running pod whose container was just restarted
// restarts times, last ended by SIGTERM (exit 143).
func killedByLiveness(restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 143, Reason: "Error",
				StartedAt:  metav1.NewTime(c.now.Add(-40 * time.Second)),
				FinishedAt: metav1.NewTime(c.now.Add(-5 * time.Second)),
			},
		}
	}
}
