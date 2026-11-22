package scenarios

import (
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The gap17 library holds the scenarios written from SCENARIO_GAPS_17.md:
// one root in a shared or cluster-scoped place makes workloads of other
// namespaces fail. These helpers keep each story short.

// g17Obj builds an unstructured object, namespaced or cluster-scoped.
func g17Obj(
	c *cluster, api, kind, namespace, name string, body map[string]any,
) *unstructured.Unstructured {
	meta := clusterMeta(c, namespace, name, strings.ToLower(kind))
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": api, "kind": kind}}
	for key, value := range body {
		u.Object[key] = value
	}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	return u
}

// g17FQDN is the in-cluster name of a Service of another namespace.
func g17FQDN(c *cluster, service, namespace string) string {
	return c.n(service) + "." + c.n(namespace) + ".svc.cluster.local"
}

// g17Up lists a healthy Deployment of n pods on n1 and its Service.
func g17Up(c *cluster, namespace, name string, n int) *workload {
	w := c.deployment(namespace, name,
		"registry.example.com/"+name+":1", int32(n))
	c.list(w.objects())
	pods := make([]*corev1.Pod, n)
	for i := range pods {
		pods[i] = w.pod(i, "n1")
		c.list(pods[i])
	}
	c.list(clusterService(c, namespace, name, 8080),
		clusterSlice(c, namespace, name, pods...))
	return w
}

// g17Apps lists a healthy Deployment of n pods for every "namespace/name".
func g17Apps(c *cluster, n int, apps ...string) []*workload {
	var out []*workload
	for _, app := range apps {
		namespace, name, _ := strings.Cut(app, "/")
		out = append(out, g17Up(c, namespace, name, n))
	}
	return out
}

// g17State puts the first k pods of w in state and updates its status.
func g17State(c *cluster, w *workload, k int, state podState) {
	for i := range k {
		c.update(w.pod(i, "n1", state))
	}
	w.setReady(*w.deployment.Spec.Replicas - int32(k))
	c.update(w.objects())
}

// g17Crash crash-loops every pod of w with message.
func g17Crash(c *cluster, w *workload, restarts int32, message string) {
	g17State(c, w, int(*w.deployment.Spec.Replicas),
		crashLoop(1, "Error", message, restarts))
}

// g17Victims crash-loops every app, round by round, with message.
func g17Victims(
	c *cluster, apps []*workload, round int32, message string,
) {
	for _, app := range apps {
		g17Crash(c, app, round, message)
	}
}

// g17Unready runs the pods of w without readiness; the kubelet reports
// the failed probe with message.
func g17Unready(c *cluster, w *workload, round int32, message string) {
	n := int(*w.deployment.Spec.Replicas)
	g17State(c, w, n, notReady)
	gapKubelet(c, w.pod(0, "n1"), "Unhealthy", message, round)
}

// g17Mount keeps the pods of w in ContainerCreating on a failed mount.
func g17Mount(c *cluster, w *workload, round int32, message string) {
	n := int(*w.deployment.Spec.Replicas)
	g17State(c, w, n, gapPending(waiting("ContainerCreating", "")))
	gapKubelet(c, w.pod(0, "n1"), "FailedMount", message, round)
}

// g17Loop runs fn once a minute for rounds rounds.
func g17Loop(c *cluster, rounds int32, fn func(round int32)) {
	for round := int32(1); round <= rounds; round++ {
		fn(round)
		c.after(time.Minute)
	}
}

// g17Sts lists a one-pod StatefulSet on node and its Service.
func g17Sts(
	c *cluster, namespace, name, node string,
) *appsv1.StatefulSet {
	meta := c.meta(namespace, name)
	labels := map[string]string{"app": meta.Name}
	one := int32(1)
	sts := &appsv1.StatefulSet{ObjectMeta: meta,
		Spec: appsv1.StatefulSetSpec{Replicas: &one,
			ServiceName: meta.Name,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels,
				"registry.example.com/"+name+":1")},
		Status: appsv1.StatefulSetStatus{Replicas: 1, ReadyReplicas: 1}}
	sts.Generation, sts.Status.ObservedGeneration = 1, 1
	pod := operatorBroker(c, sts, 0, node)
	c.list(sts, pod, clusterService(c, namespace, name, 5432),
		clusterSlice(c, namespace, name, pod))
	return sts
}

// g17Ingress is an Ingress of namespace routing to the Service backend.
func g17Ingress(
	c *cluster, namespace, name, backend string,
) *networkingv1.Ingress {
	prefix := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: clusterMeta(c, namespace, name, "ingress"),
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			Host: name + ".example.com",
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path: "/", PathType: &prefix,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: c.n(backend),
								Port: networkingv1.ServiceBackendPort{
									Number: 8080}}}}}}}}}},
	}
}

// g17FailedCreate plays ReplicaSets whose pods the API server refuses:
// each Deployment reports ReplicaFailure and its ReplicaSet warns.
func g17FailedCreate(c *cluster, apps []*workload, message string) {
	for _, w := range apps {
		d := w.deployment.DeepCopy()
		d.Status.UpdatedReplicas = 0
		d.Status.Conditions = []appsv1.DeploymentCondition{{
			Type: appsv1.DeploymentReplicaFailure, Status: "True",
			Reason: "FailedCreate", Message: message,
			LastTransitionTime: metav1.NewTime(c.now)}}
		c.update(d)
	}
	g17Loop(c, 8, func(round int32) {
		for _, w := range apps {
			g2Event(c, w.replicaSet, "ReplicaSet", "FailedCreate",
				message, "replicaset-controller", round)
		}
	})
}
