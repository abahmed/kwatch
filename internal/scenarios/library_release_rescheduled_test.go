package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// releaseRescheduledScenarios are pods of an old revision recreated by a
// node rotation: new pods, but no new rollout.
func releaseRescheduledScenarios() []scenario {
	return []scenario{releaseOldRevisionRescheduled()}
}

// releaseOldRevisionRescheduled: revision 14 of a Deployment has run for
// three weeks. Its nodes are rotated, so its four pods are recreated on
// a new node, and each restarts twice while it warms up. The pods are
// new, the revision is not: there is no release to compare.
func releaseOldRevisionRescheduled() scenario {
	return scenario{
		expect: expectation{
			Name: "release-old-revision-rescheduled",
			Description: "The pods of a three-week-old revision are " +
				"recreated by a node rotation and restart twice each.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "checkout",
				"registry.example.com/checkout:5.1", 4)
			w.replicaSet.Annotations = map[string]string{
				revisionAnnotation: "13"}
			c.list(w.objects())
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image =
					"registry.example.com/checkout:5.2"
			})
			rs.Annotations = map[string]string{revisionAnnotation: "14"}
			rs.CreationTimestamp = metav1.NewTime(
				c.now.Add(-21 * 24 * time.Hour))
			c.update(w.deployment)
			c.create(rs)
			c.after(5 * time.Minute)
			moved := c.now
			c.create(releasePods(w, 4, startedNow)...)
			w.setReady(0)
			c.update(w.objects())
			c.after(30 * time.Second)
			w.setReady(4)
			c.update(w.objects())
			c.after(time.Minute)
			c.update(releasePods(w, 4, createdAt(moved), restarted(1))...)
			c.after(90 * time.Second)
			c.update(releasePods(w, 4, createdAt(moved), restarted(2))...)
		},
	}
}
