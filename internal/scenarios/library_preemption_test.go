package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// preemptionScenarios are pods the scheduler removed for a pod of
// higher priority.
func preemptionScenarios() []scenario {
	return []scenario{preemptedByImporter(), preemptedAndRecovered()}
}

// preemptedByImporter: a batch importer of priority 1000 takes the room
// of two of the three worker replicas (priority 0). The replacements
// cannot be scheduled. The importer is the root of one incident.
func preemptedByImporter() scenario {
	return scenario{
		expect: expectation{
			Name: "preempted-by-higher-priority",
			Description: "Two replicas of a worker are preempted for a " +
				"higher-priority batch importer and cannot be " +
				"rescheduled.",
			Root: "deployment/batch/importer", Tier: "notify",
			MaxMessages:  4,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildPreemption(c, false) },
	}
}

// preemptedAndRecovered: the same preemption, but the replacements are
// scheduled at once and the worker is whole again. Nothing is reported.
func preemptedAndRecovered() scenario {
	return scenario{
		expect: expectation{
			Name: "preempted-and-rescheduled",
			Description: "Two worker replicas are preempted and their " +
				"replacements run again within a minute.",
			Quiet: true,
			Tail:  duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildPreemption(c, true) },
	}
}

// withPriority sets the pod's resolved priority.
func withPriority(priority int32) podState {
	return func(_ *cluster, pod *corev1.Pod) {
		pod.Spec.Priority = &priority
	}
}

// preemptedFor marks a pod as the scheduler's victim for preemptor
// ("namespace/name"), as the DisruptionTarget condition says.
func preemptedFor(preemptor string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		pod.Status.Phase = corev1.PodFailed
		now := metav1.NewTime(c.now)
		pod.Status.Conditions = append(pod.Status.Conditions,
			corev1.PodCondition{
				Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue,
				Reason: "PreemptionByScheduler", LastTransitionTime: now,
				Message: preemptor + ": preempting to accommodate a " +
					"higher priority pod",
			})
	}
}

func buildPreemption(c *cluster, recovers bool) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	worker := c.deployment("shop", "worker",
		"registry.example.com/worker:1", 3)
	importer := c.deployment("batch", "importer",
		"registry.example.com/importer:1", 1)
	c.list(worker.objects())
	c.list(importer.objects())
	c.list(worker.pod(0, "n1", withPriority(0)),
		worker.pod(1, "n1", withPriority(0)),
		worker.pod(2, "n2", withPriority(0)))
	c.after(time.Minute)
	boss := importer.pod(0, "n1", withPriority(1000))
	c.list(boss)
	name := boss.Namespace + "/" + boss.Name
	c.update(worker.pod(0, "n1", withPriority(0), preemptedFor(name)),
		worker.pod(1, "n1", withPriority(0), preemptedFor(name)))
	worker.setReady(1)
	c.update(worker.deployment, worker.replicaSet)
	c.after(30 * time.Second)
	if recovers {
		c.list(worker.pod(3, "n2", withPriority(0)),
			worker.pod(4, "n2", withPriority(0)))
		worker.setReady(3)
		c.update(worker.deployment, worker.replicaSet)
	} else {
		stuck := "0/2 nodes are available: 2 Insufficient cpu."
		c.list(worker.pod(3, "", withPriority(0),
			pendingUnscheduled(stuck)),
			worker.pod(4, "", withPriority(0), pendingUnscheduled(stuck)))
	}
	c.after(2 * time.Minute)
}
