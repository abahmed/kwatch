package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// startRuleScenarios lock how a program that stops at start is told: the
// fatal line is the cause, a probe kill is only its symptom, a binary
// built for another CPU is named outright, and the quiet between crashes
// of a container without a readiness probe is no recovery.
func startRuleScenarios() []scenario {
	return []scenario{
		startErrorRelease("start-error-release-liveness-137", 137),
		startErrorRelease("start-error-release-liveness-143", 143),
		execFormatNoRelease(), crashLoopNoReadinessProbe(),
	}
}

const startErrorLine = "Unhandled exception. Config.ConfigurationException: " +
	"The configuration is invalid: setting 'Queue:License' must be " +
	"specified."

// startErrorRelease: a release's new pod logs an unhandled configuration
// exception at start, then hangs until its liveness probe kills it (exit
// 137 or 143), round after round, while the previous version's two pods
// keep serving. The cause is the release with that line, and it stays the
// cause on every evaluation: no update about the probe's timing.
func startErrorRelease(name string, code int32) scenario {
	return scenario{
		expect: expectation{
			Name: name,
			Description: "A release's new pod stops at start with an " +
				"unhandled configuration exception and its liveness " +
				"probe kills it, while the previous version still " +
				"serves: the line is the cause, not the probe.",
			Root: "deployment/shop/orders", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildStartErrorRelease(c, code) },
	}
}

func buildStartErrorRelease(c *cluster, code int32) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "orders",
		"registry.example.com/orders:1.0", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(5 * time.Minute)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/orders:1.1"
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/health", Port: intstr.FromInt(8080)}},
			InitialDelaySeconds: 30, PeriodSeconds: 10, FailureThreshold: 5,
		}
	})
	c.update(w.deployment)
	c.create(rs)
	probe := "Liveness probe failed: Get \"http://10.244.0.31:8080/" +
		"health\": dial tcp 10.244.0.31:8080: connect: connection refused"
	for restarts := int32(1); restarts <= 8; restarts++ {
		pod := w.pod(2, "n1", killedAfterStartError(code, restarts))
		ev := c.warningEvent(pod, "Pod", "Unhealthy", probe, "kubelet",
			restarts*5)
		ev.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(ev)
		c.update(pod)
		c.after(75 * time.Second)
	}
}

// killedAfterStartError is a container running again, not ready, after a
// run that logged startErrorLine and then was killed with code.
func killedAfterStartError(code, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.State = corev1.ContainerState{Running: &corev1.
			ContainerStateRunning{StartedAt: metav1.NewTime(
			c.now.Add(-2 * time.Second))}}
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: code, Reason: "Error", Message: startErrorLine,
				StartedAt:  metav1.NewTime(c.now.Add(-73 * time.Second)),
				FinishedAt: metav1.NewTime(c.now.Add(-3 * time.Second)),
			},
		}
	}
}

// execFormatNoRelease: the only replica of a Deployment, on an arm64
// node, exits at once with "exec format error" for half an hour. No
// release is behind it and nothing serves. Its CPU is named outright,
// and the quiet between two crashes is no recovery.
func execFormatNoRelease() scenario {
	return scenario{
		expect: expectation{
			Name: "exec-format-error-no-release",
			Description: "A single replica on an arm64 node exits at " +
				"start with \"exec format error\", with no release " +
				"behind it: the image's CPU is named, it is no flap.",
			Root: "deployment/shop/reports", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			arm := c.node("n1", "zone-a")
			arm.Labels["kubernetes.io/arch"] = "arm64"
			c.list(arm)
			w := c.deployment("shop", "reports",
				"registry.example.com/reports:3.2", 1)
			crashLoopFor(c, w, "exec /usr/bin/sh: exec format error",
				255, 30*time.Minute, 0)
		},
	}
}

// crashLoopNoReadinessProbe: the only replica of a Deployment without a
// readiness probe exits with an error two minutes into every run for
// half an hour. Between crashes its container looks ready, since nothing
// checks it: that is the gap of a crash loop, not a recovery, so the
// incident stays open and is told once.
func crashLoopNoReadinessProbe() scenario {
	return scenario{
		expect: expectation{
			Name: "crash-loop-ready-between-crashes",
			Description: "A container without a readiness probe crashes " +
				"two minutes into every run for half an hour and looks " +
				"ready in between: one open incident, no flapping.",
			Root: "deployment/shop/ledger", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "ledger",
				"registry.example.com/ledger:4.0", 1)
			crashLoopFor(c, w, "fatal: lost connection to journal",
				1, 30*time.Minute, 2*time.Minute)
		},
	}
}

// crashLoopFor crashes w's only pod with message and code for span: each
// run lasts run (ready meanwhile, as without a readiness probe, when run
// is set), then the container waits out a back-off.
func crashLoopFor(
	c *cluster, w *workload, message string, code int32,
	span, run time.Duration,
) {
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	c.after(time.Minute)
	end := c.now.Add(span)
	for restarts := int32(1); c.now.Before(end); restarts++ {
		w.setReady(0)
		c.update(w.objects())
		c.update(w.pod(0, "n1", crashLoop(code, "Error", message,
			restarts)))
		c.after(40 * time.Second)
		if run == 0 {
			continue
		}
		w.setReady(1)
		c.update(w.objects())
		c.update(w.pod(0, "n1", runningAgain(code, message, restarts)))
		c.after(run)
	}
}

// runningAgain is a container running and ready again after a crash
// that ended with code and message.
func runningAgain(code int32, message string, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.Ready, status.Started = true, boolPtr(true)
		status.State = corev1.ContainerState{Running: &corev1.
			ContainerStateRunning{StartedAt: metav1.NewTime(c.now)}}
		exit := lastExit(c, message)
		exit.Terminated.ExitCode = code
		status.LastTerminationState = exit
		for i := range pod.Status.Conditions {
			pod.Status.Conditions[i].Status = corev1.ConditionTrue
		}
	}
}
