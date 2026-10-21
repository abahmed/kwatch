package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// coupling is a Deployment that reads a ConfigMap through its
// environment and gets a new image and an edit of that ConfigMap thirty
// seconds apart: the new revision's pods are exactly those that read
// the new content.
type coupling struct {
	namespace, name string
	// key is the edited ConfigMap key, from before to after.
	key, before, after string
	// image is the major version the old image carries.
	image string
	// secret reads and edits a Secret instead of a ConfigMap.
	secret bool
}

// coupledChange lists the Deployment on two nodes with its ConfigMap,
// edits the ConfigMap, rolls out the next image and crashes both
// replicas with trace.
func coupledChange(c *cluster, k coupling, trace string) {
	obj, edit := coupledObject(c, k)
	repo := "registry.example.com/" + k.name + ":"
	w := c.deployment(k.namespace, k.name, repo+k.image+".3", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].EnvFrom = coupledEnvFrom(k)
	})
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), obj)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(10 * time.Minute)
	edit(c)
	c.after(30 * time.Second)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = repo + k.image + ".4"
	})
	c.update(w.deployment)
	c.create(rs)
	c.create(w.pod(0, "n1", startedNow, notReady))
	c.after(40 * time.Second)
	for _, restarts := range []int32{2, 4, 6} {
		for i := range 2 {
			c.update(w.pod(i, "n"+itoa(i+1), startedNow,
				crashLoop(1, "Error", trace, restarts)))
		}
		w.setReady(0)
		c.update(w.objects())
		c.after(90 * time.Second)
	}
}

// coupledObject lists the object the Deployment reads and returns the
// edit that changes its key to after, by bob.
func coupledObject(c *cluster, k coupling) (runtime.Object, func(*cluster)) {
	if k.secret {
		secret := configSecret(c, k.namespace, k.name+"-secret", k.key)
		return secret, func(c *cluster) {
			edited := last(c, secret)
			edited.Data[k.key] = []byte(k.after)
			editedBy(c, edited, "bob")
			c.update(edited)
		}
	}
	cm := configMap(c, k.namespace, k.name+"-config",
		map[string]string{"currency": "EGP", k.key: k.before})
	return cm, func(c *cluster) {
		edited := last(c, cm)
		edited.Data[k.key] = k.after
		editedBy(c, edited, "bob")
		c.update(edited)
	}
}

// coupledEnvFrom makes the container read the object's keys.
func coupledEnvFrom(k coupling) []corev1.EnvFromSource {
	if k.secret {
		return []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: k.name + "-secret"}}}}
	}
	return []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: k.name + "-config"}}}}
}

// coupledConfigFaults: the config loader rejects the edited value. The
// edit is the root; the rollout it shares pods with is not.
func coupledConfigFaults() scenario {
	return scenario{
		expect: expectation{
			Name: "coupled-config-rejected-value",
			Description: "A new image and an edited ConfigMap arrive " +
				"together and the pods stop on a YAML error from " +
				"the loader.",
			Root: "configmap/billing/ledger-config", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			coupledChange(c, coupling{namespace: "billing",
				name: "ledger", key: "rates.yaml",
				before: "usd: 1.0", after: "usd: [1.0", image: "5"},
				"fatal: failed to parse rates.yaml: yaml: line 1: did "+
					"not find expected ',' or ']'")
		},
	}
}

// coupledImageFaults: the same pair, but the crash is a nil pointer in
// code that only the new image has; the edit changed a log level.
func coupledImageFaults() scenario {
	return scenario{
		expect: expectation{
			Name: "coupled-image-nil-pointer",
			Description: "A new image and an unrelated ConfigMap edit " +
				"arrive together and the pods crash with a nil " +
				"pointer in new code.",
			Root: "deployment/stock/sync", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			coupledChange(c, coupling{namespace: "stock", name: "sync",
				key: "log_level", before: "info", after: "debug",
				image: "3"},
				"panic: runtime error: invalid memory address or nil "+
					"pointer dereference at sync/reconcile.go:131")
		},
	}
}

// coupledTwins are scenarios in pairs with the same observations and
// different truths: a new image and an edited ConfigMap or Secret land
// together and the pods stop with an error that names neither. Nothing
// kwatch can see tells a pair apart, so its answer is right for one and
// wrong for the other; they measure how often a likely answer is right
// when the evidence is genuinely open.
func coupledTwins() []scenario {
	var out []scenario
	for _, f := range []struct {
		suffix, trace string
		k             coupling
	}{
		{"", "fatal: startup failed (exit status 1)", coupling{
			name: "intake", key: "mode", before: "batch",
			after: "stream", image: "4"}},
		{"-secret", "fatal: startup failed (exit status 1)", coupling{
			name: "relay", key: "token", after: "rotated",
			image: "6", secret: true}},
		{"-state", "panic: invariant violated: queue not empty at " +
			"startup", coupling{name: "ledger", key: "mode",
			before: "strict", after: "lenient", image: "9"}},
	} {
		out = append(out, coupledTwin(f.suffix, f.trace, f.k, true),
			coupledTwin(f.suffix, f.trace, f.k, false))
	}
	return out
}

// coupledTwin is one half of a pair: image says whether the new image
// or the edited object is the truth.
func coupledTwin(
	suffix, trace string, k coupling, image bool,
) scenario {
	kind, object, culprit := "config", "configmap", "mode is unsupported"
	if k.secret {
		kind, object, culprit = "secret", "secret", "token is refused"
	}
	name, namespace := "coupled-silent-"+kind+"-fault"+suffix, "post"
	root := object + "/" + namespace + "/" + k.name + "-" + kind
	story := "A new image and an edited " + kind + " land together " +
		"and the pods exit without saying why; the new " + culprit + "."
	if image {
		name = "coupled-silent-image-fault" + suffix
		namespace = "mail"
		root = "deployment/" + namespace + "/" + k.name
		story = "A new image and an edited " + kind + " land " +
			"together and the pods exit without saying why; the " +
			"new image dropped a flag."
	}
	k.namespace = namespace
	return scenario{
		expect: expectation{
			Name: name, Description: story, Root: root, Tier: "notify",
			MaxMessages: 2,
			// Nothing in the cluster tells the image from the config:
			// naming both as possible causes is the honest answer.
			Ambiguous:    true,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) { coupledChange(c, k, trace) },
	}
}
