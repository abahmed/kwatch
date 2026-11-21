package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The gap2 and gap3 libraries hold scenarios written from the gap lists
// SCENARIO_GAPS_2.md and SCENARIO_GAPS_3.md. Their labels come from what
// Kubernetes does, not from what kwatch answered.

// g2Exp is a label with the usual budget of two messages.
func g2Exp(name, desc, root, tier string, not ...string) expectation {
	return expectation{Name: name, Description: desc, Root: root,
		Tier: tier, MaxMessages: 2, MustNotBlame: not}
}

// g2Quiet is the label of a story that must stay silent.
func g2Quiet(name, desc string) expectation {
	return expectation{Name: name, Description: desc, Quiet: true}
}

// tail sets how long the replay runs after the last observation.
func (e expectation) tail(d time.Duration) expectation {
	e.Tail = duration(d)
	return e
}

// budget sets the number of messages the scenario may produce.
func (e expectation) budget(n int) expectation {
	e.MaxMessages = n
	return e
}

// g2Event delivers a Warning event about obj.
func g2Event(
	c *cluster, obj metav1.Object, kind, reason, message, source string,
	count int32,
) {
	c.warn(c.warningEvent(obj, kind, reason, message, source, count))
}

// g2NodeCondition sets a condition on node name and delivers the update.
func g2NodeCondition(
	c *cluster, name string, kind corev1.NodeConditionType,
	status corev1.ConditionStatus, reason, message string,
) *corev1.Node {
	node := last(c, c.node(name, "zone-a"))
	setNodeCondition(node, kind, status, reason, message, c.now)
	c.update(node)
	return node
}

// g2Release builds a scenario in which a release of the shop Deployment
// api changes its pod spec and run plays what the new pods do.
func g2Release(
	e expectation, mutate func(*cluster, *corev1.PodSpec),
	run func(*cluster, *workload),
) scenario {
	return scenario{expect: e, build: func(c *cluster) {
		w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
			mutate(c, spec)
		})
		run(c, w)
	}}
}

// g2Crash plays six restarts of the new pod, 45 seconds apart.
func g2Crash(exit int32, reason, message string) func(*cluster, *workload) {
	return func(c *cluster, w *workload) {
		gapRounds(c, w, 6, 45*time.Second, func(round int32) podState {
			return crashLoop(exit, reason, message, round)
		}, nil)
	}
}

// g2Wait plays a new pod that cannot start: it waits in reason and the
// kubelet reports event with message every round.
func g2Wait(reason, event, message string) func(*cluster, *workload) {
	return func(c *cluster, w *workload) {
		gapRounds(c, w, 6, 45*time.Second, func(int32) podState {
			return gapPending(waiting(reason, message))
		}, func(pod *corev1.Pod, round int32) {
			gapKubelet(c, pod, event, message, round)
		})
	}
}

// g2Unready plays a new pod that runs but never becomes ready; the kubelet
// reports a failed probe every round.
func g2Unready(message string) func(*cluster, *workload) {
	return func(c *cluster, w *workload) {
		gapRounds(c, w, 8, 30*time.Second, func(int32) podState {
			return notReady
		}, func(pod *corev1.Pod, round int32) {
			gapKubelet(c, pod, "Unhealthy", message, round)
		})
	}
}

// g2Pending plays a new pod the scheduler cannot place.
func g2Pending(message string) func(*cluster, *workload) {
	return func(c *cluster, w *workload) {
		c.create(w.pod(0, "", pendingUnscheduled(message)))
		pendingForMinutes(c, w, message, 8)
	}
}

// g2FailedCreate plays a ReplicaSet whose pods the API server refuses: its
// Deployment reports ReplicaFailure and the ReplicaSet warns every minute.
func g2FailedCreate(message string) func(*cluster, *workload) {
	return func(c *cluster, w *workload) {
		d := w.deployment.DeepCopy()
		d.Status.UpdatedReplicas = 0
		d.Status.Conditions = []appsv1.DeploymentCondition{{
			Type: appsv1.DeploymentReplicaFailure, Status: "True",
			Reason: "FailedCreate", Message: message,
			LastTransitionTime: metav1.NewTime(c.now),
		}}
		c.update(d)
		for m := int32(1); m <= 8; m++ {
			g2Event(c, w.replicaSet, "ReplicaSet", "FailedCreate", message,
				"replicaset-controller", m)
			c.after(time.Minute)
		}
	}
}

// g2Daemon lists a DaemonSet with a ready pod on every node named.
func g2Daemon(
	c *cluster, namespace, name string, nodes ...string,
) *appsv1.DaemonSet {
	labels := map[string]string{"app": c.n(name)}
	ds := &appsv1.DaemonSet{
		ObjectMeta: c.meta(namespace, name),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/"+name+":1"),
		},
	}
	ds.Generation, ds.Status.ObservedGeneration = 1, 1
	setAgentCounts(ds, int32(len(nodes)), int32(len(nodes)))
	c.list(ds)
	for _, node := range nodes {
		c.list(agentPod(c, ds, node))
	}
	return ds
}

// g2Failed is a pod the kubelet rejected: it never started.
func g2Failed(reason, message string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		now := metav1.NewTime(c.now)
		pod.CreationTimestamp = now
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodFailed, Reason: reason, Message: message,
			StartTime: &now,
		}
	}
}
