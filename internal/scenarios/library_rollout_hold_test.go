package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// rolloutHoldScenarios are rollouts that take their time. A healthy
// one is not news, however long it keeps replicas below the desired
// count; one that stalls is.
func rolloutHoldScenarios() []scenario {
	return []scenario{
		{
			expect: expectation{
				Name: "rollout-healthy-slow",
				Description: "A four-replica Deployment rolls out one " +
					"pod at a time, each taking over two minutes to " +
					"become ready. One replica is short for nine " +
					"minutes, as the rollout intends.",
				Quiet: true, Tail: duration(20 * time.Minute),
			},
			build: func(c *cluster) { slowRollout(c, "catalog", false) },
		},
		{
			expect: expectation{
				Name: "rollout-stalled",
				Description: "A Deployment's new revision never passes " +
					"its readiness probe: nothing crashes, but the " +
					"rollout stops and the controller reports that it " +
					"missed its progress deadline.",
				Root: "deployment/shop/ledger", Tier: "notify",
				MaxMessages: 3, MustNotBlame: []string{"node//n1"},
				Tail: duration(20 * time.Minute),
			},
			build: func(c *cluster) { slowRollout(c, "ledger", true) },
		},
	}
}

// slowRollout rolls the shop Deployment name over to a new image, one
// pod at a time. Each new pod waits two and a half minutes for its
// readiness probe. When stalls is false it then passes and the next pod
// starts; when true the first new pod never passes and, ten minutes in,
// the controller reports ProgressDeadlineExceeded.
func slowRollout(c *cluster, name string, stalls bool) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", name, "registry.example.com/"+name+":9", 4)
	c.list(w.objects())
	var old []*corev1.Pod
	for i := 0; i < 4; i++ {
		old = append(old, w.pod(i, "n"+itoa(i%2+1)))
	}
	c.list(old[0], old[1], old[2], old[3])
	c.after(5 * time.Minute)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/" + name + ":10"
	})
	rollProgress(w, c, appsv1.DeploymentProgressing, "ReplicaSetUpdated")
	c.update(w.deployment)
	c.create(rs)
	rolled := c.now
	for i := 0; i < 4; i++ {
		started := c.now
		c.remove(old[i])
		c.create(w.pod(i, "n"+itoa(i%2+1), startedNow, notReady))
		w.deployment.Status.UpdatedReplicas = int32(i + 1)
		w.deployment.Status.ReadyReplicas = 3
		w.deployment.Status.AvailableReplicas = 3
		w.deployment.Status.UnavailableReplicas = 1
		c.update(w.deployment)
		if stalls {
			c.after(10*time.Minute - c.now.Sub(rolled))
			rollProgress(w, c, appsv1.DeploymentProgressing,
				"ProgressDeadlineExceeded")
			c.update(w.deployment)
			c.after(15 * time.Minute)
			return
		}
		c.after(150 * time.Second)
		c.update(w.pod(i, "n"+itoa(i%2+1), createdAt(started)))
	}
	w.setReady(4)
	rollProgress(w, c, appsv1.DeploymentProgressing,
		"NewReplicaSetAvailable")
	c.update(w.deployment)
}

// rollProgress sets the Deployment's Progressing condition to reason,
// as the controller does while it rolls.
func rollProgress(
	w *workload, c *cluster, kind appsv1.DeploymentConditionType,
	reason string,
) {
	status := corev1.ConditionTrue
	if reason == "ProgressDeadlineExceeded" {
		status = corev1.ConditionFalse
	}
	d := w.deployment.DeepCopy()
	d.Status.Conditions = []appsv1.DeploymentCondition{
		{Type: kind, Status: status, Reason: reason,
			LastTransitionTime: metav1.NewTime(c.now),
			LastUpdateTime:     metav1.NewTime(c.now)},
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue,
			Reason:             "MinimumReplicasAvailable",
			LastTransitionTime: metav1.NewTime(c.start)},
	}
	w.deployment = d
}
