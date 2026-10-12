package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// namespaceDeleteScenarios cover what a namespace deletion does to the
// pods that still run in it.
func namespaceDeleteScenarios() []scenario {
	return []scenario{namespaceDeleteCrashingPods()}
}

// namespaceDeleteCrashingPods: a worker crash-loops in a preview
// namespace; the namespace is deleted, its ConfigMap and ServiceAccount
// go first and the pods follow. What the pods referenced vanishes by
// design: the crash stays the incident, and neither object is blamed
// nor reported missing.
func namespaceDeleteCrashingPods() scenario {
	return scenario{
		expect: expectation{
			Name: "namespace-delete-crashing-pods",
			Description: "Pods crash-loop in a namespace that is then " +
				"deleted; the ConfigMap and ServiceAccount they use " +
				"disappear before the pods do.",
			Root: "deployment/preview-4/mailer", Tier: "notify",
			MaxMessages: 3,
			MustNotBlame: []string{"configmap/preview-4/mailer-settings",
				"serviceaccount/preview-4/default"},
			Tail: duration(10 * time.Minute),
		},
		build: buildNamespaceDelete,
	}
}

func buildNamespaceDelete(c *cluster) {
	ns := &corev1.Namespace{ObjectMeta: c.meta("", "preview-4")}
	ns.Status.Phase = corev1.NamespaceActive
	settings := &corev1.ConfigMap{
		ObjectMeta: c.meta("preview-4", "mailer-settings"),
		Data:       map[string]string{"smtp": "relay"},
	}
	account := &corev1.ServiceAccount{
		ObjectMeta: c.meta("preview-4", "default")}
	w := c.deployment("preview-4", "mailer",
		"registry.example.com/mailer:5", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.ServiceAccountName = "default"
		spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: settings.Name}}}}
	})
	c.list(c.node("n1", "zone-a"), ns, settings, account)
	w.setReady(0)
	c.list(w.objects())
	crash := crashLoop(1, "Error", "fatal: cannot reach the relay", 5)
	c.list(w.pod(0, "n1", crash), w.pod(1, "n1", crash))
	c.after(8 * time.Minute)
	terminating := last(c, ns)
	deleting(&terminating.ObjectMeta, c.now)
	terminating.Status.Phase = corev1.NamespaceTerminating
	c.update(terminating)
	c.remove(settings, account)
	c.after(time.Minute)
	c.remove(w.pod(0, "n1", crash), w.pod(1, "n1", crash))
	c.after(time.Minute)
}
