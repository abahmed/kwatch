package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// revisionRollbackScenarios are bad releases of StatefulSets and
// DaemonSets, whose history lives in ControllerRevisions: a rollback
// goes to the revision before the newest, by number, and only when a
// ready pod of that revision shows it served.
func revisionRollbackScenarios() []scenario {
	return []scenario{
		{expect: expectation{
			Name: "statefulset-release-rollback",
			Description: "A StatefulSet rolls out an image whose first " +
				"replaced pod crash-loops; the pods still on the " +
				"previous revision keep serving.",
			Root: "statefulset/streaming/queue", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n2"},
		}, build: func(c *cluster) { buildRevisionRelease(c, 3, 3) }},
		{expect: expectation{
			Name: "statefulset-release-previous-unseen",
			Description: "A one-replica StatefulSet rolls out an image " +
				"that crash-loops; no pod of the previous revision is " +
				"left to show it served, so no rollback is suggested.",
			Root: "statefulset/streaming/queue", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n2"},
		}, build: func(c *cluster) { buildRevisionRelease(c, 1, 0) }},
		{expect: expectation{
			Name: "daemonset-gitops-release",
			Description: "A DaemonSet a GitOps controller deploys rolls " +
				"out an image whose new pod crash-loops while the pod " +
				"of the previous revision serves; the undo is a change " +
				"in Git.",
			Root: "daemonset/ops/agent", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n2"},
		}, build: buildGitOpsDaemonSetRelease},
	}
}

// buildRevisionRelease rolls a StatefulSet of replicas from revision 1
// to revision 2; the highest ordinal, replaced first, crash-loops.
func buildRevisionRelease(c *cluster, replicas, ready int32) {
	roll := rollRevision(c, replicas, ready)
	crashRelease(c, func(states ...podState) *corev1.Pod {
		return fromRevision(operatorBroker(c, roll.rolled,
			int(replicas-1), "n2", states...), roll.after)
	})
}

// revisionRoll is a StatefulSet before and after its template change,
// with the hashes of its old and new revision.
type revisionRoll struct {
	sts, rolled   *appsv1.StatefulSet
	before, after string
}

// rollRevision lists a StatefulSet of replicas at revision 1 with
// ready pods of it (ready of them, the rest gone, so with none kwatch
// never sees that revision serve), and five minutes later changes its
// image, creating revision 2.
func rollRevision(c *cluster, replicas, ready int32) revisionRoll {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	meta := clusterMeta(c, "streaming", "queue", "sts")
	labels := map[string]string{"app": meta.Name}
	sts := &appsv1.StatefulSet{ObjectMeta: meta,
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas, ServiceName: meta.Name,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/queue:4"),
		}}
	before, after := meta.Name+"-7c5d", meta.Name+"-9f2b"
	sts.Generation, sts.Status.ObservedGeneration = 1, 1
	sts.Status.Replicas, sts.Status.ReadyReplicas = ready, ready
	sts.Status.AvailableReplicas = ready
	sts.Status.CurrentReplicas, sts.Status.UpdatedReplicas = ready, ready
	sts.Status.CurrentRevision, sts.Status.UpdateRevision = before, before
	c.list(sts, revisionOf(sts, "StatefulSet", before, 1))
	for i := range int(ready) {
		c.list(fromRevision(operatorBroker(c, sts, i, "n1"), before))
	}
	c.after(5 * time.Minute)
	rolled := sts.DeepCopy()
	rolled.Generation, rolled.Status.ObservedGeneration = 2, 2
	rolled.Spec.Template.Spec.Containers[0].Image =
		"registry.example.com/queue:5"
	rolled.Status.UpdateRevision, rolled.Status.UpdatedReplicas = after, 1
	rolled.Status.ReadyReplicas = max(ready-1, 0)
	rolled.Status.AvailableReplicas = max(ready-1, 0)
	c.update(rolled)
	c.create(revisionOf(rolled, "StatefulSet", after, 2))
	return revisionRoll{sts: sts, rolled: rolled, before: before,
		after: after}
}

// buildGitOpsDaemonSetRelease rolls a DaemonSet that a GitOps
// controller deploys to a new image; the pod on n2 is replaced first
// and crash-loops, the one on n1 still runs the previous revision.
func buildGitOpsDaemonSetRelease(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	labels := map[string]string{"app": "agent"}
	ds := &appsv1.DaemonSet{ObjectMeta: c.meta("ops", "agent"),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/agent:2"),
		}}
	ds.Labels = map[string]string{"argocd.argoproj.io/instance": "ops"}
	ds.Generation, ds.Status.ObservedGeneration = 1, 1
	setAgentCounts(ds, 2, 2)
	c.list(ds, revisionOf(ds, "DaemonSet", ds.Name+"-5b8f", 1))
	for _, node := range []string{"n1", "n2"} {
		c.list(fromRevision(agentPod(c, ds, node), "5b8f"))
	}
	c.after(5 * time.Minute)
	rolled := ds.DeepCopy()
	rolled.Generation, rolled.Status.ObservedGeneration = 2, 2
	rolled.Spec.Template.Spec.Containers[0].Image =
		"registry.example.com/agent:3"
	setAgentCounts(rolled, 2, 1)
	rolled.Status.UpdatedNumberScheduled = 1
	c.update(rolled)
	c.create(revisionOf(rolled, "DaemonSet", ds.Name+"-d41a", 2))
	crashRelease(c, func(states ...podState) *corev1.Pod {
		return fromRevision(agentPod(c, rolled, "n2", states...), "d41a")
	})
}

// crashRelease replaces a pod with the new revision's, which starts and
// then crash-loops for ten minutes.
func crashRelease(c *cluster, pod func(...podState) *corev1.Pod) {
	c.update(pod(startedNow, notReady))
	c.after(40 * time.Second)
	c.update(pod(startedNow, crashLoop(1, "Error",
		"panic: unknown flag --queue-size", 3)))
	c.after(2 * time.Minute)
	c.update(pod(startedNow, crashLoop(1, "Error",
		"panic: unknown flag --queue-size", 5)))
	c.after(10 * time.Minute)
}

// revisionOf is the ControllerRevision name, numbered revision, of the
// workload owner of kind.
func revisionOf(
	owner metav1.Object, kind, name string, revision int64,
) *appsv1.ControllerRevision {
	yes := true
	return &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: name,
			Namespace: owner.GetNamespace(), UID: types.UID(name + "-cr"),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: kind, Name: owner.GetName(),
				UID: owner.GetUID(), Controller: &yes}}},
		Data:     runtime.RawExtension{Raw: []byte(`{}`)},
		Revision: revision,
	}
}

// fromRevision labels pod as made from the revision hash names.
func fromRevision(pod *corev1.Pod, hash string) *corev1.Pod {
	labels := map[string]string{kube.LabelRevisionHash: hash}
	for k, v := range pod.Labels {
		labels[k] = v
	}
	pod.Labels = labels
	return pod
}
