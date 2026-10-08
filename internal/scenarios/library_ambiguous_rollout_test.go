package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Ambiguous rollout scenarios and the job-failure helper.

// ambiguousRollout rolls payments (three replicas on three nodes) to a
// new image while n1 reports memory pressure. The new replica on n1
// crashes; the others crash too when everyNode is set.
func ambiguousRollout(c *cluster, everyNode bool) {
	n1 := c.node("n1", "zone-a")
	setNodeCondition(n1, corev1.NodeMemoryPressure, corev1.ConditionTrue,
		"KubeletHasInsufficientMemory", "memory is short",
		c.start.Add(5*time.Minute))
	c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
	w := c.deployment("shop", "payments",
		"registry.example.com/payments:5.1", 3)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n3"))
	c.after(5 * time.Minute)
	pressure := last(c, n1)
	setNodeCondition(pressure, corev1.NodeMemoryPressure,
		corev1.ConditionTrue, "KubeletHasInsufficientMemory",
		"memory is short", c.now)
	c.update(pressure)
	c.after(time.Minute)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/payments:5.2"
	})
	c.update(w.deployment)
	c.create(rs)
	for i := range 3 {
		c.create(w.pod(i, "n"+itoa(i+1), startedNow, notReady))
	}
	c.after(40 * time.Second)
	message := "fatal: cannot allocate memory"
	if everyNode {
		message = "panic: nil pointer dereference in ledger.Open()"
	}
	for _, restarts := range []int32{2, 4, 6} {
		for i := range 3 {
			if i > 0 && !everyNode {
				c.update(w.pod(i, "n"+itoa(i+1)))
				continue
			}
			c.update(w.pod(i, "n"+itoa(i+1), startedNow,
				crashLoop(1, "Error", message, restarts)))
		}
		w.setReady(map[bool]int32{true: 0, false: 2}[everyNode])
		c.update(w.objects())
		c.after(90 * time.Second)
	}
}

// ambiguousRolloutOnePressuredNode: only the replica on the node under
// memory pressure fails; the new revision runs fine elsewhere.
func ambiguousRolloutOnePressuredNode() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-rollout-one-pressured-node",
			Description: "A rollout while one node is under memory " +
				"pressure; only the new replica on that node fails.",
			Root: "node//n1", Tier: "notify", MaxMessages: 3,
		},
		build: func(c *cluster) { ambiguousRollout(c, false) },
	}
}

// ambiguousRolloutEveryNode: the same rollout and pressure, but the new
// replicas fail on every node with an error of their own.
func ambiguousRolloutEveryNode() scenario {
	return scenario{
		expect: expectation{
			Name: "ambiguous-rollout-every-node",
			Description: "A rollout while one node is under memory " +
				"pressure; the new replicas fail on every node.",
			Root: "deployment/shop/payments", Tier: "notify",
			MaxMessages:  3,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) { ambiguousRollout(c, true) },
	}
}

// ambiguousJobFailed is a Job that reached its backoff limit at at.
func ambiguousJobFailed(at time.Time) []batchv1.JobCondition {
	return []batchv1.JobCondition{{
		Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
		Reason:             "BackoffLimitExceeded",
		Message:            "Job has reached the backoff limit",
		LastTransitionTime: metav1.NewTime(at),
	}}
}
