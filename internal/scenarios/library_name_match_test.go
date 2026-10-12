package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// nameMatchScenarios keep the proof that an error names a short-named
// object when it quotes the name. The negative side (the name inside
// another word is not a proof) is a scorer test: the edit alone still
// roots the incident at the ConfigMap, with less confidence.
func nameMatchScenarios() []scenario {
	return []scenario{shortNameQuotedInError()}
}

// shortNameQuotedInError: a ConfigMap called "api" is edited two
// minutes before a worker crash-loops with an error that quotes the
// ConfigMap by name: the edit is the cause.
func shortNameQuotedInError() scenario {
	return scenario{
		expect: expectation{
			Name: "short-name-quoted-in-error",
			Description: "A ConfigMap named like a common word is edited " +
				"just before crashes whose text quotes that name.",
			Root: "configmap/pipeline/api", Tier: "notify",
			MaxMessages: 3, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			buildShortNameCrash(c,
				"fatal: configmap \"api\": limits.yaml: invalid quota")
		},
	}
}

func buildShortNameCrash(c *cluster, crash string) {
	cm := configMap(c, "pipeline", "api",
		map[string]string{"limits.yaml": "quota: 100"})
	w := c.deployment("pipeline", "ingest",
		"registry.example.com/ingest:3.4", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Volumes = []corev1.Volume{{Name: "limits",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cm.Name}}}}}
	})
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), cm)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(2 * time.Minute)
	edited := last(c, cm)
	edited.Data["limits.yaml"] = "quota: 250"
	c.update(edited)
	c.after(2 * time.Minute)
	for _, restarts := range []int32{2, 4} {
		for i := range 2 {
			c.update(w.pod(i, "n"+itoa(i+1),
				crashLoop(1, "Error", crash, restarts)))
		}
		w.setReady(0)
		c.update(w.deployment, w.replicaSet)
		c.after(2 * time.Minute)
	}
}
