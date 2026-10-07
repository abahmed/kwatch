package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// configVersionScenarios are failures where pods read different content
// of one ConfigMap: environment is read when a container starts, so the
// pods that started before an edit and the ones after it differ.
func configVersionScenarios() []scenario {
	return []scenario{
		configEditedNewPodsFail(), configEditedPlainMount(),
		configVersionsMixedDigest(),
	}
}

// configEditedNewPodsFail: an edit puts a bad value in the ConfigMap the
// api pods load as environment. The three running pods keep the old
// environment; the two replicas added after the edit read the new value
// and crash-loop. The ConfigMap edit is the cause, and the message says
// which pods fail.
func configEditedNewPodsFail() scenario {
	return scenario{
		expect: expectation{
			Name: "config-edited-new-pods-fail",
			Description: "A ConfigMap edit breaks the environment of the " +
				"two replicas started after it; the three older pods " +
				"keep the old environment and stay healthy.",
			Root: "configmap/shop/api-config", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api", "node//n1",
				"node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) {
			configEditedBuild(c, func(spec *corev1.PodSpec, name string) {
				spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: name}}}}
			})
		},
	}
}

// configEditedPlainMount: the same edit and the same two failing
// replicas, but the ConfigMap is a plain volume mount, which the kubelet
// updates in place in every pod. No pod is stuck on old content, so the
// message must not claim a version split.
func configEditedPlainMount() scenario {
	return scenario{
		expect: expectation{
			Name: "config-edited-plain-mount",
			Description: "A ConfigMap edit breaks the two replicas " +
				"started after it; the config is a plain volume mount " +
				"that updates in place, so no version split is claimed.",
			Root: "configmap/shop/api-config", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api", "node//n1",
				"node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) {
			configEditedBuild(c, func(spec *corev1.PodSpec, name string) {
				spec.Volumes = []corev1.Volume{{Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: name}}}}}
			})
		},
	}
}

// configEditedBuild runs the story both scenarios share: five api
// replicas use api-config as wired by use, an edit lands, and two
// replicas created after it crash-loop.
func configEditedBuild(
	c *cluster, use func(spec *corev1.PodSpec, name string),
) {
	cm := configMap(c, "shop", "api-config",
		map[string]string{"DB_HOST": "db-1", "TIMEOUT": "30"})
	w := c.deployment("shop", "api", "registry.example.com/api:3.4", 5)
	configTemplate(w, func(spec *corev1.PodSpec) { use(spec, cm.Name) })
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), cm)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n1"))
	c.after(90 * time.Second)
	edited := last(c, cm)
	edited.Data["TIMEOUT"] = "thirty"
	c.update(edited)
	c.after(30 * time.Second)
	crash := "config: TIMEOUT: cannot parse \"thirty\" as a number"
	for _, restarts := range []int32{2, 4} {
		for i := 3; i < 5; i++ {
			c.update(w.pod(i, "n"+itoa(i%2+1), createdAt(c.start.Add(
				2*time.Minute)), crashLoop(1, "Error", crash, restarts)))
		}
		w.setReady(3)
		c.update(w.deployment, w.replicaSet)
		c.after(2 * time.Minute)
	}
}

// configVersionsMixedDigest: the api ConfigMap is edited and two
// replicas the autoscaler adds afterwards read the new content, while the
// three older pods keep the old environment. Nothing fails, and nobody
// restarts the old pods. After half an hour that is a configuration risk
// for the digest; it never opens an incident, so the scenario stays quiet.
func configVersionsMixedDigest() scenario {
	return scenario{
		expect: expectation{
			Name: "config-versions-mixed-digest",
			Description: "Pods started before and after a ConfigMap edit " +
				"run side by side for a long time, none failing: " +
				"advice for the digest, never an interruption.",
			Quiet: true, Tail: duration(45 * time.Minute),
		},
		build: func(c *cluster) {
			cm := configMap(c, "shop", "api-config",
				map[string]string{"DB_HOST": "db-1"})
			w := c.deployment("shop", "api", "registry.example.com/api:3.4", 5)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cm.Name}}}}
			})
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), cm)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n1"))
			c.after(time.Minute)
			edited := last(c, cm)
			edited.Data["DB_HOST"] = "db-2"
			c.update(edited)
			c.after(time.Minute)
			c.update(w.pod(3, "n2", startedNow), w.pod(4, "n1", startedNow))
			c.update(w.deployment, w.replicaSet)
		},
	}
}
