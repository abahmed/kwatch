package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// revisionAnnotation is where a Deployment records a ReplicaSet's
// revision number.
const revisionAnnotation = "deployment.kubernetes.io/revision"

// releaseScenarios are releases judged against the revision they
// replaced: a worse one is reported, an equal one is not.
func releaseScenarios() []scenario {
	return []scenario{
		releaseScenario("release-regression",
			"Rollout 14 of a Deployment replaces a stable rollout 13. "+
				"Its four pods stay running and ready but restart twice "+
				"each within three minutes; rollout 13 never restarted.",
			2),
		releaseScenario("release-steady",
			"Rollout 14 of a Deployment replaces rollout 13. One of its "+
				"four pods restarts once, as one pod of rollout 13 did "+
				"now and then: nothing worse than before.",
			1),
	}
}

// releaseScenario builds a rollout whose pods restart each time restarts
// is reached. The release-steady twin restarts one pod once, which is
// too little to judge; the regression restarts every pod twice.
func releaseScenario(name, description string, restarts int32) scenario {
	expect := expectation{Name: name, Description: description}
	if restarts > 1 {
		expect.Root = "deployment/shop/checkout"
		expect.Tier = "notify"
		expect.MaxMessages = 2
	} else {
		expect.Quiet = true
		expect.Tail = duration(30 * time.Minute)
	}
	return scenario{expect: expect, build: func(c *cluster) {
		c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
		w := c.deployment("shop", "checkout",
			"registry.example.com/checkout:5.1", 4)
		w.replicaSet.Annotations = map[string]string{revisionAnnotation: "13"}
		c.list(w.objects())
		old := releasePods(w, 4)
		c.list(old...)
		c.after(5 * time.Minute)
		rs := w.rollout(func(spec *corev1.PodSpec) {
			spec.Containers[0].Image = "registry.example.com/checkout:5.2"
		})
		rs.Annotations = map[string]string{revisionAnnotation: "14"}
		c.update(w.deployment)
		c.create(rs)
		rolled := c.now
		c.create(releasePods(w, 4, startedNow)...)
		c.after(time.Minute)
		c.remove(old...)
		c.after(30 * time.Second)
		first := 1
		if restarts > 1 {
			first = 4
		}
		c.update(releasePods(w, first, createdAt(rolled),
			restarted(1))...)
		c.after(90 * time.Second)
		if restarts > 1 {
			c.update(releasePods(w, 4, createdAt(rolled),
				restarted(restarts))...)
		}
	}}
}

// releasePods builds the first n replicas of the workload's current
// ReplicaSet, spread over two nodes.
func releasePods(
	w *workload, n int, states ...podState,
) []runtime.Object {
	var pods []runtime.Object
	for i := 0; i < n; i++ {
		pods = append(pods, w.pod(i, "n"+itoa(i%2+1), states...))
	}
	return pods
}

// restarted is a pod that stays running and ready but whose container
// has been restarted count times, the last one half a minute ago.
func restarted(count int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = count
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 1, Reason: "Error",
				StartedAt:  metav1.NewTime(c.now.Add(-2 * time.Minute)),
				FinishedAt: metav1.NewTime(c.now.Add(-30 * time.Second)),
			},
		}
	}
}
