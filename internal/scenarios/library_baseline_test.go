package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// baselineScenarios compare a workload with its own last hours. The
// same restarts are news from a service that never restarts and noise
// from a worker that always does.
func baselineScenarios() []scenario {
	return []scenario{
		{
			expect: expectation{
				Name: "stable-restarts-spike",
				Description: "A Deployment that has not restarted for " +
					"twelve hours has one container restart six times " +
					"in forty minutes. The message sets that against " +
					"its usual none.",
				Root: "deployment/shop/orders", Tier: "notify",
				MaxMessages: 3, Tail: duration(20 * time.Minute),
			},
			build: stableRestartsSpike,
		},
		{
			expect: expectation{
				Name: "batch-restarts-normal",
				Description: "A worker restarts twice an hour, as it " +
					"has for thirteen hours. Its restarts are within " +
					"its normal, so only the digest carries them, " +
					"with that normal shown.",
				Root: "deployment/shop/batch-worker", Tier: "digest",
				MaxMessages: 2, Tail: duration(time.Hour),
			},
			build: batchRestartsNormal,
		},
	}
}

// stableRestartsSpike keeps the orders pods quiet for twelve hours, then
// restarts one container twice every seven minutes, ending at six.
func stableRestartsSpike(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "orders", "registry.example.com/orders:6", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n1"))
	c.after(12 * time.Hour)
	for restarts := int32(2); restarts <= 6; restarts += 2 {
		c.update(w.pod(0, "n1", restartedAgo(restarts, 30*time.Second)))
		c.after(7 * time.Minute)
	}
}

// batchRestartsNormal restarts the worker's container every half hour
// for thirteen hours. Until the last hour each restart is reported with
// the finish time of a poll that came late, so no restart looks recent
// and nothing is raised while the workload's habit is still being
// learned; the last one is fresh.
func batchRestartsNormal(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "batch-worker",
		"registry.example.com/batch-worker:3", 1)
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	restarts := int32(0)
	for i := 0; i < 26; i++ {
		c.after(30 * time.Minute)
		restarts++
		ago := 20 * time.Minute
		if i >= 24 {
			ago = 30 * time.Second
		}
		c.update(w.pod(0, "n1", restartedAgo(restarts, ago)))
	}
}

// restartedAgo is a ready pod whose container has been restarted count
// times, the last one finishing ago before now.
func restartedAgo(count int32, ago time.Duration) podState {
	return func(c *cluster, pod *corev1.Pod) {
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = count
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 1, Reason: "Error",
				StartedAt:  metav1.NewTime(c.now.Add(-ago - 10*time.Minute)),
				FinishedAt: metav1.NewTime(c.now.Add(-ago)),
			},
		}
	}
}
