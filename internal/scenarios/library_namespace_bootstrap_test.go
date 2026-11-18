package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// namespaceBootstrapScenarios cover the objects Kubernetes publishes by
// itself into every new namespace (the kube-root-ca.crt ConfigMap and
// the default ServiceAccount) a moment after creating it.
func namespaceBootstrapScenarios() []scenario {
	return []scenario{newNamespaceCrashBeforeRootCA()}
}

// rootCAName is the ConfigMap the control plane publishes into every
// namespace for the service-account token volume. Its name is fixed.
const rootCAName = "kube-root-ca.crt"

// newNamespaceCrashBeforeRootCA: a Deployment is applied together with a
// brand-new namespace. Its pod mounts the usual service-account token
// volume, which projects kube-root-ca.crt, and starts before kwatch sees
// that ConfigMap and the default ServiceAccount: Kubernetes publishes
// them a moment after the namespace, and on a busy API server their
// watch events reach kwatch a minute and a half later.
// The pod then crash-loops for a reason of its own. Neither bootstrap
// object is missing: it is an ordering effect, never the cause.
func newNamespaceCrashBeforeRootCA() scenario {
	return scenario{
		expect: expectation{
			Name: "new-namespace-crash-before-root-ca",
			Description: "A pod in a namespace created seconds ago " +
				"crash-loops for its own reason before kwatch sees the " +
				"namespace's kube-root-ca.crt ConfigMap and default " +
				"ServiceAccount.",
			Root: "deployment/team-new/api", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"configmap/team-new/" + rootCAName,
				"serviceaccount/team-new/default"},
			Tail: duration(10 * time.Minute),
		},
		build: buildNewNamespaceCrash,
	}
}

func buildNewNamespaceCrash(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	c.after(time.Minute)
	ns := &corev1.Namespace{ObjectMeta: c.meta("", "team-new")}
	ns.CreationTimestamp = metav1.NewTime(c.now)
	ns.Status.Phase = corev1.NamespaceActive
	c.create(ns)
	w := c.deployment("team-new", "api", "registry.example.com/api:1", 1)
	configTemplate(w, tokenVolume)
	w.setReady(0)
	w.deployment.CreationTimestamp = ns.CreationTimestamp
	c.create(w.deployment, w.replicaSet)
	born := createdAt(c.now)
	c.create(w.pod(0, "n1", startedNow, notReady))
	message := "fatal: flag --listen-port is required"
	c.after(15 * time.Second)
	c.update(w.pod(0, "n1", born, terminatedState(message, 1)))
	c.after(15 * time.Second)
	c.update(w.pod(0, "n1", born, crashLoop(1, "Error", message, 1)))
	c.after(time.Minute)
	rootCA := &corev1.ConfigMap{ObjectMeta: c.meta("team-new", "x"),
		Data: map[string]string{"ca.crt": "-----BEGIN CERTIFICATE-----"}}
	rootCA.Name = rootCAName
	rootCA.CreationTimestamp = metav1.NewTime(c.now.Add(-89 * time.Second))
	account := &corev1.ServiceAccount{ObjectMeta: c.meta("team-new",
		"default")}
	account.CreationTimestamp = rootCA.CreationTimestamp
	c.create(rootCA, account)
	for restarts := int32(2); restarts <= 4; restarts++ {
		c.after(time.Minute)
		c.update(w.pod(0, "n1", born, startedState(message, restarts)))
		c.after(20 * time.Second)
		c.update(w.pod(0, "n1", born, crashLoop(1, "Error", message,
			restarts)))
		c.after(3 * time.Minute)
	}
}

// tokenVolume gives the pod the service-account token volume Kubernetes
// adds to every pod: a token, the cluster CA from kube-root-ca.crt and
// the namespace name.
func tokenVolume(spec *corev1.PodSpec) {
	spec.ServiceAccountName = "default"
	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name: "kube-api-access", VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
						Path: "token"}},
					{ConfigMap: &corev1.ConfigMapProjection{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: rootCAName}}},
				}}}})
}
