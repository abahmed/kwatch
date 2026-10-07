package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// graceKillScenarios are pods that did not stop in time when they were
// terminated.
func graceKillScenarios() []scenario {
	return []scenario{rolloutHardKills(), rolloutHardKillsDigest(),
		rolloutCleanStops()}
}

// rolloutHardKills: a rollout replaces three api pods; the old ones
// ignore SIGTERM and are killed (exit 137) when their 30 second grace
// period ends, and the new pods cannot start. The users see an outage,
// so the incident notifies.
func rolloutHardKills() scenario {
	return scenario{
		expect: expectation{
			Name: "rollout-hard-kills",
			Description: "Three api pods ignore SIGTERM during a " +
				"rollout and are killed when their grace period " +
				"ends; the new pods crash without a word.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildGraceRollout(c, 137, true, true) },
	}
}

// rolloutHardKillsDigest: the same hard kills in a healthy rollout. No
// user impact: the incident waits for the digest and sends no message.
func rolloutHardKillsDigest() scenario {
	return scenario{
		expect: expectation{
			Name: "rollout-hard-kills-healthy",
			Description: "Three api pods are killed at the end of their " +
				"grace period in a rollout that succeeds; nobody is " +
				"interrupted.",
			Quiet: true,
			Tail:  duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildGraceRollout(c, 137, true, false) },
	}
}

// rolloutCleanStops: the same rollout, but the old pods exit 0 at once.
// Nothing is reported.
func rolloutCleanStops() scenario {
	return scenario{
		expect: expectation{
			Name: "rollout-clean-stops",
			Description: "Old api pods stop cleanly when a rollout " +
				"replaces them.",
			Quiet: true,
			Tail:  duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildGraceRollout(c, 0, false, false) },
	}
}

// terminatedAtGrace is a pod deleted a grace period ago whose container
// ended with exit at the deadline (immediately when late is false).
func terminatedAtGrace(exit int32, late bool) podState {
	return func(c *cluster, pod *corev1.Pod) {
		grace := int64(30)
		pod.Spec.TerminationGracePeriodSeconds = &grace
		deadline := c.now
		end := c.now
		if !late {
			end = c.now.Add(-28 * time.Second)
		}
		stamp := metav1.NewTime(deadline)
		pod.DeletionTimestamp = &stamp
		pod.DeletionGracePeriodSeconds = &grace
		notReady(c, pod)
		for i := range pod.Status.ContainerStatuses {
			pod.Status.ContainerStatuses[i].State = corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode: exit, Reason: "Error",
					FinishedAt: metav1.NewTime(end),
				},
			}
		}
	}
}

func buildGraceRollout(c *cluster, exit int32, late, broken bool) {
	nodes := []string{"n1", "n2", "n3"}
	for _, name := range nodes {
		c.list(c.node(name, "zone-a"))
	}
	api := c.deployment("shop", "api", "registry.example.com/api:1", 3)
	c.list(api.objects())
	var old []*corev1.Pod
	for i, node := range nodes {
		pod := api.pod(i, node)
		old = append(old, pod)
		c.list(pod)
	}
	c.after(time.Minute)
	newRS := api.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/api:2"
	})
	api.deployment.Status.UpdatedReplicas = 1
	c.update(api.deployment)
	c.list(newRS)
	for i, node := range nodes {
		if broken {
			c.list(api.pod(3+i, node, startedNow,
				crashLoop(1, "Error", "", 3)))
			continue
		}
		c.list(api.pod(3+i, node, startedNow))
	}
	c.after(31 * time.Second)
	for _, pod := range old {
		c.update(withStates(c, pod, terminatedAtGrace(exit, late)))
	}
	api.deployment.Status.UpdatedReplicas = 3
	if broken {
		api.deployment.Status.ReadyReplicas = 0
		api.deployment.Status.AvailableReplicas = 0
		api.deployment.Status.UnavailableReplicas = 3
	}
	c.update(api.deployment)
	// The pods linger while the kubelet tears down their volumes and
	// network, as they do after a real termination.
	c.after(100 * time.Second)
	for _, pod := range old {
		c.remove(pod)
	}
	c.after(2 * time.Minute)
}

// withStates applies pod states to pod and returns it.
func withStates(c *cluster, pod *corev1.Pod, states ...podState) *corev1.Pod {
	for _, state := range states {
		state(c, pod)
	}
	return pod
}
