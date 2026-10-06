package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// firstRolloutScenarios are new workloads that never worked.
func firstRolloutScenarios() []scenario {
	return []scenario{firstRolloutNeverHealthy()}
}

// firstRolloutNeverHealthy: a new Deployment's only pod runs but never
// passes its readiness probe. It has had one ReplicaSet and has never
// been available, so the message says it never became healthy.
func firstRolloutNeverHealthy() scenario {
	return scenario{
		expect: expectation{
			Name: "first-rollout-never-healthy",
			Description: "A new Deployment's first rollout runs but " +
				"never becomes ready.",
			Root: "deployment/shop/web", Tier: "notify", MaxMessages: 3,
			Tail: duration(20 * time.Minute),
		},
		build: buildFirstRollout,
	}
}

func buildFirstRollout(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "web", "registry.example.com/web:1.0", 1)
	w.deployment.CreationTimestamp = metav1.NewTime(c.now)
	w.replicaSet.CreationTimestamp = metav1.NewTime(c.now)
	w.setReady(0)
	w.deployment.Status.Conditions = []appsv1.DeploymentCondition{{
		Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse,
		Reason:             "MinimumReplicasUnavailable",
		LastTransitionTime: metav1.NewTime(c.now),
	}}
	c.list(w.objects())
	probe := "Readiness probe failed: HTTP probe failed with statuscode: 503"
	pod := w.pod(0, "n1", startedNow, notReady)
	c.list(pod)
	for count := int32(3); count <= 9; count += 3 {
		c.after(5 * time.Minute)
		c.warn(c.warningEvent(pod, "Pod", "Unhealthy", probe, "kubelet",
			count))
	}
}
