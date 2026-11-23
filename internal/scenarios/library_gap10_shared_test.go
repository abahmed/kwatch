package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap10Scenarios are the scenarios written from SCENARIO_GAPS_10.md
// (messy observation: lost, late, skewed or hidden evidence) and
// SCENARIO_GAPS_18.md (partial outages: one zone, pool, replica, port or
// tenant).
func gap10Scenarios() []scenario {
	var out []scenario
	for _, group := range [][]scenario{
		gap10aScenarios(), gap10bScenarios(),
		gap18aScenarios(), gap18bScenarios(),
	} {
		out = append(out, group...)
	}
	return out
}

// g10Fleet lists the nodes, a Deployment of replicas ready pods spread
// over them and returns the workload. mutate edits the pod template
// before anything is listed.
func g10Fleet(
	c *cluster, ns, name string, replicas int, nodes []string,
	mutate func(*corev1.PodSpec),
) *workload {
	for _, node := range nodes {
		c.list(c.node(node, "zone-a"))
	}
	w := c.deployment(ns, name, "registry.example.com/"+name+":1.0",
		int32(replicas))
	if mutate != nil {
		configTemplate(w, mutate)
	}
	c.list(w.objects())
	for i := 0; i < replicas; i++ {
		c.list(w.pod(i, nodes[i%len(nodes)]))
	}
	return w
}

// g10Settle sets the ready count of w and delivers its objects.
func g10Settle(c *cluster, w *workload, ready int32) {
	w.setReady(ready)
	c.update(w.deployment, w.replicaSet)
}

// g10Event delivers a Warning event about pod stamped at, which may
// differ from the delivery time (replays, skewed clocks).
func g10Event(
	c *cluster, pod *corev1.Pod, reason, message, source string,
	count int32, at time.Time,
) {
	ev := c.warningEvent(pod, "Pod", reason, message, source, count)
	ev.InvolvedObject.UID = pod.UID
	ev.FirstTimestamp = metav1.NewTime(at)
	ev.LastTimestamp = metav1.NewTime(at)
	c.warn(ev)
}

// g10Sts lists a StatefulSet and one ready pod per node named.
func g10Sts(
	c *cluster, ns, name string, nodes []string, mutate func(*corev1.PodSpec),
) *appsv1.StatefulSet {
	meta := clusterMeta(c, ns, name, "sts")
	replicas := int32(len(nodes))
	labels := map[string]string{"app": meta.Name}
	sts := &appsv1.StatefulSet{
		ObjectMeta: meta,
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas, ServiceName: meta.Name,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/"+name+":1"),
		},
	}
	if mutate != nil {
		mutate(&sts.Spec.Template.Spec)
	}
	sts.Generation, sts.Status.ObservedGeneration = 1, 1
	sts.Status.Replicas, sts.Status.ReadyReplicas = replicas, replicas
	sts.Status.AvailableReplicas, sts.Status.UpdatedReplicas = replicas,
		replicas
	c.list(sts)
	for i, node := range nodes {
		c.list(operatorBroker(c, sts, i, node))
	}
	return sts
}

// g10Lost is a pod on a node that stopped reporting: still bound, not
// ready, marked for deletion by the node controller.
func g10Lost(c *cluster, pod *corev1.Pod) {
	notReady(c, pod)
	pod.Status.Reason = "NodeLost"
	now := metav1.NewTime(c.now)
	pod.DeletionTimestamp = &now
}

// g10Secret mounts the named Secret as a volume of the pod spec.
func g10Secret(name string) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) {
		spec.Volumes = append(spec.Volumes, corev1.Volume{Name: "creds",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.
				SecretVolumeSource{SecretName: name}}})
	}
}

// g10Ago is the time d before the cluster's current time.
func g10Ago(c *cluster, d time.Duration) time.Time { return c.now.Add(-d) }
