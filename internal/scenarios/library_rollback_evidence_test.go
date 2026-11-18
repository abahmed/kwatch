package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// rollbackEvidenceScenarios check the evidence behind a rollback: a
// revision kwatch saw serving before a completed rollout scaled it to
// zero is still a rollback target, and a StatefulSet release whose
// previous revision fails the same way is not blamed.
func rollbackEvidenceScenarios() []scenario {
	return []scenario{
		{expect: expectation{
			Name: "release-after-completed-rollout",
			Description: "Revision 14 of a Deployment rolls out, serves " +
				"and replaces revision 13; later revision 15 replaces " +
				"it all at once and crash-loops. Revision 14 has no " +
				"pod left, but kwatch saw it serve, so the rollback " +
				"to it is suggested.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(20 * time.Minute),
		}, build: buildReleaseAfterCompletedRollout},
		{expect: expectation{
			Name: "statefulset-release-previous-fails",
			Description: "A StatefulSet rolls out an image whose first " +
				"replaced pod crash-loops, and a pod still on the " +
				"previous revision, restarted, stops with the same " +
				"line: the release is not what broke it.",
			Root: "statefulset/streaming/queue", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1", "node//n2"},
		}, build: buildStatefulSetPreviousFails},
	}
}

// buildReleaseAfterCompletedRollout: checkout runs one replica of
// revision 13; revision 14 rolls out and completes (revision 13 at
// zero, its pod gone); half an hour later revision 15 is rolled out the
// Recreate way, so revision 14 goes to zero and its pod is removed
// before the new pod crash-loops.
func buildReleaseAfterCompletedRollout(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "checkout",
		"registry.example.com/checkout:5.1", 1)
	w.deployment.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType
	w.replicaSet.Annotations = map[string]string{revisionAnnotation: "13"}
	c.list(w.objects())
	pod := w.pod(0, "n1")
	c.list(pod)
	c.after(5 * time.Minute)
	recreate(c, w, pod, "14", "registry.example.com/checkout:5.2")
	created := c.now
	c.create(w.pod(0, "n2", startedNow, notReady))
	c.after(time.Minute)
	pod = w.pod(0, "n2", createdAt(created))
	c.update(pod)
	w.setReady(1)
	c.update(w.replicaSet)
	c.update(w.deployment)
	c.after(30 * time.Minute)
	recreate(c, w, pod, "15", "registry.example.com/checkout:5.3")
	releaseUndoCrashLoop(c, w)
}

// recreate rolls the workload to a numbered revision with image the
// Recreate way: its current ReplicaSet goes to zero and pod is removed
// as the new ReplicaSet appears.
func recreate(
	c *cluster, w *workload, pod *corev1.Pod, revision, image string,
) {
	old := w.replicaSet.DeepCopy()
	zero := int32(0)
	old.Spec.Replicas = &zero
	old.Status = appsv1.ReplicaSetStatus{}
	numberedRollout(c, w, "alice", revision, func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = image
	})
	c.update(old)
	c.remove(pod)
}

// queueStartLine is the line both revisions of queue stop with.
const queueStartLine = "panic: unknown flag --queue-size"

// buildStatefulSetPreviousFails rolls a StatefulSet of three from
// revision 1 to revision 2; the replaced pod crash-loops, and pod 0,
// still on revision 1, restarts and stops with the same line from the
// same minute.
func buildStatefulSetPreviousFails(c *cluster) {
	roll := rollRevision(c, 3, 3)
	replaced := func(states ...podState) *corev1.Pod {
		return fromRevision(operatorBroker(c, roll.rolled, 2, "n2",
			states...), roll.after)
	}
	c.update(replaced(startedNow, notReady))
	c.after(40 * time.Second)
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.update(replaced(startedNow, crashLoop(1, "Error",
			queueStartLine, restarts+2)))
		c.update(fromRevision(operatorBroker(c, roll.sts, 0, "n1",
			crashLoop(1, "Error", queueStartLine, restarts)), roll.before))
		c.after(time.Minute)
	}
	c.after(5 * time.Minute)
}
