package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// livenessKindScenarios tell a kubelet kill for a failing liveness
// check apart from an application that crashes by itself.
func livenessKindScenarios() []scenario {
	return []scenario{livenessKillHTTP500(), livenessKillsSlowStart(),
		livenessSymptomOfCrash()}
}

// livenessKillHTTP500: the app answers its liveness check with HTTP 500
// and ignores SIGTERM, so each kill ends in SIGKILL (exit 137) without
// any memory pressure. It is the liveness check that restarts it, not
// a memory kill and not a crash.
func livenessKillHTTP500() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-kill-http-500",
			Description: "The kubelet restarts a container whose " +
				"liveness check answers HTTP 500; each kill ends in " +
				"SIGKILL after the grace period.",
			Root: "deployment/shop/catalog", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			w := probedWorkload(c, "catalog", 10)
			c.after(time.Minute)
			for restarts := int32(3); restarts <= 8; restarts++ {
				c.after(40 * time.Second)
				probeKillRound(c, w, restarts, 137,
					"Liveness probe failed: HTTP probe failed with "+
						"statuscode: 500", 30*time.Second)
			}
		},
	}
}

// livenessKillsSlowStart: a new Deployment without a startupProbe or a
// readiness probe needs about a minute to start, but its liveness probe
// gives up after 25 seconds (a 5s delay and two 10s periods), so every
// pod is killed while it starts.
func livenessKillsSlowStart() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-kills-slow-start-no-startup-probe",
			Description: "Liveness kills every pod of a Deployment " +
				"that has no startupProbe while it is still starting.",
			Root: "deployment/shop/reports", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			w := probedWorkload(c, "reports", 5)
			c.after(time.Minute)
			for restarts := int32(3); restarts <= 8; restarts++ {
				c.after(30 * time.Second)
				probeKillRound(c, w, restarts, 143,
					"Liveness probe failed: Get \"http://10.244.0.31:8080"+
						"/healthz\": dial tcp 10.244.0.31:8080: connect: "+
						"connection refused", 27*time.Second)
			}
		},
	}
}

// livenessSymptomOfCrash: the app exits by itself with an error a few
// seconds after each start because it cannot reach its broker; while it
// is down, its liveness probe is refused. The crash is the problem, the
// liveness failures only follow it.
func livenessSymptomOfCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-refused-while-app-crashes",
			Description: "A container exits with an error on its own; " +
				"its liveness probe is refused while it is down.",
			Root: "deployment/shop/notifier", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			w := probedWorkload(c, "notifier", 10)
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.after(40 * time.Second)
				probeKillRound(c, w, restarts, 1,
					"Liveness probe failed: Get \"http://10.244.0.40:8080"+
						"/healthz\": dial tcp 10.244.0.40:8080: connect: "+
						"connection refused", 6*time.Second)
			}
		},
	}
}

// probedWorkload is shop/name with two pods on n1 and a liveness probe
// with the given initial delay, two failures 10 seconds apart and no
// startup or readiness probe.
func probedWorkload(c *cluster, name string, delay int32) *workload {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", name, "registry.example.com/"+name+":2", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Ports = []corev1.ContainerPort{{
			Name: "http", ContainerPort: 8080}}
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz", Port: intstr.FromInt32(8080)}},
			InitialDelaySeconds: delay, PeriodSeconds: 10,
			FailureThreshold: 2,
		}
	})
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n1"))
	return w
}

// probeKillRound records the liveness failures of every pod and the
// restart that follows: a run of run that ended with exit.
func probeKillRound(c *cluster, w *workload, restarts, exit int32,
	probe string, run time.Duration,
) {
	for i := range 2 {
		pod := w.pod(i, "n1", endedWith(restarts, exit, run))
		ev := c.warningEvent(pod, "Pod", "Unhealthy", probe, "kubelet",
			restarts*2)
		ev.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(ev)
		c.update(pod)
	}
	w.setReady(0)
	c.update(w.objects())
}

// endedWith is a container running again, not ready, after a run of
// run that ended with exit a moment ago.
func endedWith(restarts, exit int32, run time.Duration) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.State = corev1.ContainerState{Running: &corev1.
			ContainerStateRunning{StartedAt: metav1.NewTime(
			c.now.Add(-2 * time.Second))}}
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: exit, Reason: "Error",
				StartedAt:  metav1.NewTime(c.now.Add(-run - 3*time.Second)),
				FinishedAt: metav1.NewTime(c.now.Add(-3 * time.Second)),
			},
		}
	}
}
