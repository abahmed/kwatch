package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// statefulSetScaleScenarios are ordered StatefulSet scale-ups: one that
// moves pod by pod as it should, and one that stops.
func statefulSetScaleScenarios() []scenario {
	return []scenario{
		statefulSetOrderedScaleUp(), statefulSetScaleUpStalls(),
	}
}

// scaleQueue is a three-broker StatefulSet, listed ready on two nodes.
func scaleQueue(c *cluster) *appsv1.StatefulSet {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	meta := clusterMeta(c, "streaming", "queue", "sts")
	replicas := int32(3)
	labels := map[string]string{"app": meta.Name}
	sts := &appsv1.StatefulSet{
		ObjectMeta: meta,
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas, ServiceName: meta.Name,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/queue:4"),
		},
	}
	sts.Generation, sts.Status.ObservedGeneration = 1, 1
	sts.Status.Replicas, sts.Status.ReadyReplicas = 3, 3
	sts.Status.AvailableReplicas, sts.Status.CurrentReplicas = 3, 3
	sts.Status.CurrentRevision, sts.Status.UpdateRevision = "queue-a",
		"queue-a"
	sts.Status.UpdatedReplicas = 3
	c.list(sts)
	for i := range 3 {
		c.list(operatorBroker(c, sts, i, "n"+itoa(i%2+1)))
	}
	return sts
}

// scaleQueueTo asks for nine brokers.
func scaleQueueTo(c *cluster, sts *appsv1.StatefulSet) *appsv1.StatefulSet {
	scaled := sts.DeepCopy()
	replicas := int32(9)
	scaled.Spec.Replicas = &replicas
	scaled.Generation, scaled.Status.ObservedGeneration = 2, 2
	c.update(scaled)
	return scaled
}

// scaleQueueStep creates broker i, not ready yet, and after wait turns
// it ready and counts it in the status.
func scaleQueueStep(
	c *cluster, sts *appsv1.StatefulSet, i int, wait time.Duration,
) {
	node := "n" + itoa(i%2+1)
	c.create(operatorBroker(c, sts, i, node, startedNow, notReady))
	sts.Status.Replicas, sts.Status.CurrentReplicas = int32(i+1), int32(i+1)
	sts.Status.UpdatedReplicas = int32(i + 1)
	c.update(sts.DeepCopy())
	c.after(wait)
	c.update(operatorBroker(c, sts, i, node, startedNow))
	sts.Status.ReadyReplicas = int32(i + 1)
	sts.Status.AvailableReplicas = int32(i + 1)
	c.update(sts.DeepCopy())
}

// statefulSetOrderedScaleUp: a StatefulSet grows from three to nine
// brokers in order, one new broker ready every two minutes while the
// first three serve throughout. It is short of nine for twelve minutes
// and moving the whole time: nothing is wrong.
func statefulSetOrderedScaleUp() scenario {
	return scenario{
		expect: expectation{
			Name: "statefulset-ordered-scale-up",
			Description: "An OrderedReady StatefulSet scales from 3 to " +
				"9; a new pod becomes ready every two minutes and the " +
				"original three serve throughout.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			sts := scaleQueue(c)
			c.after(5 * time.Minute)
			sts = scaleQueueTo(c, sts)
			for i := 3; i < 9; i++ {
				scaleQueueStep(c, sts, i, 2*time.Minute)
			}
		},
	}
}

// statefulSetScaleUpStalls: the same scale-up stops at the fifth
// broker, which never becomes ready. Once nothing has moved for the
// stuck window the shortfall is reported.
func statefulSetScaleUpStalls() scenario {
	return scenario{
		expect: expectation{
			Name: "statefulset-scale-up-stalls",
			Description: "An OrderedReady StatefulSet scales from 3 to " +
				"9; the fifth pod never becomes ready, so the scale-up " +
				"stops with four ready.",
			Root: "statefulset/streaming/queue", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			sts := scaleQueue(c)
			c.after(5 * time.Minute)
			sts = scaleQueueTo(c, sts)
			scaleQueueStep(c, sts, 3, 2*time.Minute)
			c.create(operatorBroker(c, sts, 4, "n1", startedNow, notReady))
			sts.Status.Replicas, sts.Status.CurrentReplicas = 5, 5
			sts.Status.UpdatedReplicas = 5
			c.update(sts.DeepCopy())
			c.after(25 * time.Minute)
		},
	}
}
