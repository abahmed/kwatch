package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// crashCycleScenarios are crash loops that last for hours.
func crashCycleScenarios() []scenario {
	return []scenario{crashLoopForHours(), crashQuotesOtherError()}
}

// crashLoopForHours: the only replica of a Deployment crashes on every
// start for nearly two hours. Its configuration names a Service that does
// not exist, and a Service of its own has no endpoints. The pod walks
// the usual cycle: running for seconds, terminated with Error, then
// waiting in CrashLoopBackOff. However the pod looks at the moment of
// each look, the story is one incident, told once.
func crashLoopForHours() scenario {
	return scenario{
		expect: expectation{
			Name: "crash-loop-for-hours",
			Description: "The only replica of a Deployment crashes on " +
				"every start for nearly two hours, cycling through " +
				"running, Error and CrashLoopBackOff; it stays one " +
				"incident with one revised cause.",
			Root: "deployment/staging/warehouse", Tier: "notify",
			MaxMessages:  3,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: buildCrashLoopForHours,
	}
}

// crashQuotesOtherError: the same crash loop, but the container quotes a
// missing assembly. The Service its configuration names is not the
// cause, and the pod's own error is.
func crashQuotesOtherError() scenario {
	return scenario{
		expect: expectation{
			Name: "crash-quotes-other-error",
			Description: "A crash loop quotes a missing assembly while " +
				"the configuration names a Service that does not " +
				"exist; the workload is blamed, not the name.",
			Root: "deployment/staging/warehouse", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"service/staging/regional"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			crashLoopWithError(c, fileNotFoundLine, 12)
		},
	}
}

func buildCrashLoopForHours(c *cluster) {
	crashLoopWithError(c, "panic: no route to regional", 22)
}

// crashLoopWithError crashes the warehouse pod through cycles turns,
// each time quoting message.
func crashLoopWithError(c *cluster, message string, cycles int32) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("staging", "warehouse",
		"registry.example.com/warehouse:7", 1)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name: "REGIONAL_URL", Value: "http://" + c.n("regional") +
				"." + c.n("staging") + ".svc:8080"}}
	})
	c.list(w.objects())
	c.list(w.pod(0, "n1"), clusterService(c, "staging", "warehouse", 8080))
	c.after(time.Minute)
	for restarts := int32(1); restarts <= cycles; restarts++ {
		crashCycle(c, w, message, restarts)
	}
}

// crashCycle shows one five-minute turn of a crash loop: the pod runs
// for a few seconds, ends with Error, then waits out the back-off.
func crashCycle(
	c *cluster, w *workload, message string, restarts int32,
) {
	w.setReady(0)
	c.update(w.objects())
	c.update(w.pod(0, "n1", startedState(message, restarts)))
	c.after(20 * time.Second)
	c.update(w.pod(0, "n1", terminatedState(message, restarts)))
	c.after(30 * time.Second)
	c.update(w.pod(0, "n1", crashLoop(1, "Error",
		message, restarts)))
	c.after(4 * time.Minute)
}

// startedState is a container that just started again and is not ready.
func startedState(message string, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.Started = boolPtr(true)
		status.State = corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.NewTime(c.now)},
		}
		status.LastTerminationState = lastExit(c, message)
	}
}

// terminatedState is a container that just exited with Error.
func terminatedState(message string, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.Started = boolPtr(false)
		status.State = corev1.ContainerState{
			Terminated: lastExit(c, message).Terminated,
		}
		status.LastTerminationState = lastExit(c, message)
	}
}

// lastExit is the exit with Error that just ended the container.
func lastExit(c *cluster, message string) corev1.ContainerState {
	return corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{
			ExitCode: 1, Reason: "Error", Message: message,
			StartedAt:  metav1.NewTime(c.now.Add(-20 * time.Second)),
			FinishedAt: metav1.NewTime(c.now),
		},
	}
}
