package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// restartSemanticsScenarios are containers whose restarts newer
// Kubernetes makes on purpose: a native sidecar that is still starting,
// a restart rule on an exit code, a kubelet with a short maximum
// back-off. Each has a twin that is a real crash loop.
func restartSemanticsScenarios() []scenario {
	return []scenario{sidecarStartsSlowly(), restartRuleExitCode(),
		restartRuleOtherCode(), crashLoopShortBackoff()}
}

// sidecarStartsSlowly: a release adds a native sidecar whose startup
// probe allows fifteen minutes. The first new replica's sidecar runs
// and passes the probe after nine; until then the pod is Pending and
// its app waits, while the old replicas serve. That is a pod starting,
// not one stuck in init or Pending, so nothing is said.
func sidecarStartsSlowly() scenario {
	return scenario{
		expect: expectation{
			Name: "sidecar-starts-slowly",
			Description: "A native sidecar passes its long startup " +
				"probe after nine minutes; the pod was starting, not " +
				"stuck.",
			Quiet: true,
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "catalog",
				"registry.example.com/catalog:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(time.Minute)
			rs := w.rollout(addSlowSidecar)
			rollProgress(w, c, appsv1.DeploymentProgressing,
				"ReplicaSetUpdated")
			w.deployment.Status.UpdatedReplicas = 1
			c.update(w.deployment)
			c.create(rs)
			created := c.now
			for at := 0; at <= 9; at++ {
				c.update(w.pod(0, "n1", createdAt(created),
					sidecarNotStarted(created)))
				c.after(time.Minute)
			}
			c.update(w.pod(0, "n1", createdAt(created), sidecarRunning))
			c.after(5 * time.Minute)
		},
	}
}

// addSlowSidecar adds a native sidecar whose startup probe allows
// fifteen minutes.
func addSlowSidecar(spec *corev1.PodSpec) {
	always := corev1.ContainerRestartPolicyAlways
	spec.InitContainers = []corev1.Container{{
		Name: "agent", Image: "registry.example.com/agent:2",
		RestartPolicy: &always,
		StartupProbe:  &corev1.Probe{PeriodSeconds: 10, FailureThreshold: 90},
	}}
}

// sidecarNotStarted is a new pod whose sidecar runs since at but has not
// passed its startup probe: the pod is Pending and its app container
// waits to be initialised.
func sidecarNotStarted(at time.Time) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		pod.Status.Phase = corev1.PodPending
		agent := pod.Spec.InitContainers[0]
		pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{
			Name: agent.Name, Image: agent.Image, Started: boolPtr(false),
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.NewTime(at)}},
		}}
		app := &pod.Status.ContainerStatuses[0]
		app.Started = boolPtr(false)
		app.State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason: "PodInitializing"}}
	}
}

// restartRuleWorker is a one-replica worker whose container asks to be
// restarted by exiting 42, as its restart rule says.
func restartRuleWorker(c *cluster) *workload {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("jobs", "worker", "registry.example.com/worker:7", 1)
	always := corev1.ContainerRestartPolicyAlways
	configTemplate(w, func(spec *corev1.PodSpec) {
		app := &spec.Containers[0]
		app.RestartPolicy = &always
		app.RestartPolicyRules = []corev1.ContainerRestartRule{{
			Action: corev1.ContainerRestartRuleActionRestart,
			ExitCodes: &corev1.ContainerRestartRuleOnExitCodes{
				Operator: corev1.ContainerRestartRuleOnExitCodesOpIn,
				Values:   []int32{42}},
		}}
	})
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	return w
}

// restartRuleExitCode: the worker recycles itself every four minutes by
// exiting 42, the code its restart rule restarts. Eight restarts in half
// an hour are what the rule asks for, so nothing is said.
func restartRuleExitCode() scenario {
	return scenario{
		expect: expectation{
			Name: "restart-rule-exit-code",
			Description: "A container exits with the code its restart " +
				"rule restarts, every four minutes; no crash loop.",
			Quiet: true,
		},
		build: func(c *cluster) {
			w := restartRuleWorker(c)
			restartCycles(c, w, 42, 4*time.Minute, 10*time.Second, 8)
		},
	}
}

// restartRuleOtherCode: the same worker exits 1 seconds after each
// start. Its restart rule names only 42, so this is a crash loop.
func restartRuleOtherCode() scenario {
	return scenario{
		expect: expectation{
			Name: "restart-rule-other-code",
			Description: "A container with a restart rule for exit 42 " +
				"keeps exiting 1 on start; a crash loop.",
			Root: "deployment/jobs/worker", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			w := restartRuleWorker(c)
			restartCycles(c, w, 1, 3*time.Second, 40*time.Second, 8)
		},
	}
}

// crashLoopShortBackoff: a kubelet whose maximum back-off is ten
// seconds (crashLoopBackOff.maxContainerRestartPeriod) restarts a
// container that fails fifty seconds after each start: it restarts
// once a minute and spends most of its loop running. The loop is still
// one crash loop, not one that clears on every run.
func crashLoopShortBackoff() scenario {
	return scenario{
		expect: expectation{
			Name: "crash-loop-short-backoff",
			Description: "A container crashes once a minute on a " +
				"kubelet whose maximum back-off is ten seconds.",
			Root: "deployment/shop/reports", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "reports",
				"registry.example.com/reports:4", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			restartCycles(c, w, 1, 50*time.Second, 10*time.Second, 14)
		},
	}
}

// restartCycles drives cycles runs of the workload's only pod: each run
// lasts run and ends with code, the kubelet waits backoff and starts it
// again.
func restartCycles(c *cluster, w *workload, code int32,
	run, backoff time.Duration, cycles int32,
) {
	c.after(run)
	for r := int32(1); r <= cycles; r++ {
		c.update(w.pod(0, "n1", exitedNow(code, run, r-1)))
		w.setReady(0)
		c.update(w.objects())
		c.after(time.Second)
		c.update(w.pod(0, "n1", inBackoff(code, run, backoff, r)))
		c.after(backoff - time.Second)
		c.update(w.pod(0, "n1", startedAgain(code, run, backoff, r)))
		w.setReady(1)
		c.update(w.objects())
		c.after(run)
	}
}

// exitedNow is the pod's container just terminated with code after run.
func exitedNow(code int32, run time.Duration, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.Started = boolPtr(false)
		status.State = corev1.ContainerState{
			Terminated: exitAt(code, c.now, run)}
	}
}

// inBackoff is the container waiting backoff before its next start.
func inBackoff(code int32, run, backoff time.Duration,
	restarts int32,
) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.Started = boolPtr(false)
		status.State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: "CrashLoopBackOff",
				Message: "back-off " + backoff.String() +
					" restarting failed container",
			}}
		status.LastTerminationState = corev1.ContainerState{
			Terminated: exitAt(code, c.now.Add(-time.Second), run)}
	}
}

// startedAgain is the container running again after its back-off.
func startedAgain(code int32, run, backoff time.Duration,
	restarts int32,
) podState {
	return func(c *cluster, pod *corev1.Pod) {
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.State = corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.NewTime(c.now)}}
		status.LastTerminationState = corev1.ContainerState{
			Terminated: exitAt(code, c.now.Add(-backoff), run)}
	}
}

func exitAt(code int32, finished time.Time,
	run time.Duration,
) *corev1.ContainerStateTerminated {
	return &corev1.ContainerStateTerminated{
		ExitCode: code, Reason: "Error",
		StartedAt:  metav1.NewTime(finished.Add(-run)),
		FinishedAt: metav1.NewTime(finished),
	}
}
