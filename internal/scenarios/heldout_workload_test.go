package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// heldoutWorkloadScenarios are held-out failures of one application or
// its configuration. See heldoutLibrary for the rule they follow.
func heldoutWorkloadScenarios() []scenario {
	return []scenario{
		heldoutConfigMapKeyMissing(), heldoutImageTagMissing(),
		heldoutSidecarCrash(),
	}
}

// heldoutConfigMapKeyMissing: someone renames the LOG_LEVEL key of the
// api-settings ConfigMap to log-level. The running pods keep the value
// they started with; when one replica is replaced, the kubelet cannot
// build the new container's environment and reports
// CreateContainerConfigError naming the missing key. The ConfigMap edit
// is the root.
func heldoutConfigMapKeyMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-configmap-key-missing",
			Description: "A ConfigMap key the pods read through " +
				"configMapKeyRef is renamed; the replica that replaces a " +
				"deleted pod fails with CreateContainerConfigError.",
			Root: "configmap/catalog/api-settings", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2",
				"deployment/catalog/api"},
		},
		build: buildHeldoutConfigMapKey,
	}
}

func buildHeldoutConfigMapKey(c *cluster) {
	cm := configMap(c, "catalog", "api-settings",
		map[string]string{"LOG_LEVEL": "info", "CACHE_TTL": "300"})
	w := c.deployment("catalog", "api", "registry.example.com/catalog:9.1",
		3)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name: "LOG_LEVEL", ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cm.Name,
					},
					Key: "LOG_LEVEL",
				},
			},
		}}
	})
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), cm)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n1"))
	c.after(3 * time.Minute)
	edited := last(c, cm)
	delete(edited.Data, "LOG_LEVEL")
	edited.Data["log-level"] = "debug"
	c.update(edited)
	c.after(4 * time.Minute)
	c.remove(last(c, w.pod(2, "")))
	w.setReady(2)
	c.update(w.objects())
	message := "couldn't find key LOG_LEVEL in ConfigMap " +
		cm.Namespace + "/" + cm.Name
	replacement := w.pod(3, "n2", startedNow,
		waiting("CreateContainerConfigError", message))
	c.create(replacement)
	for n := int32(1); n <= 4; n++ {
		ev := c.warningEvent(replacement, "Pod", "Failed",
			"Error: "+message, "kubelet", n)
		ev.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(ev)
		c.after(time.Minute)
	}
}

// heldoutImageTagMissing: release 2.4.0 rolls out to two Deployments at
// once. CI pushed the frontend image but its backend job failed, so the
// registry has no backend:2.4.0. The frontend rollout completes; the
// backend's new pod cannot pull. The backend rollout is the root, not
// the registry, which serves the frontend fine.
func heldoutImageTagMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-image-tag-missing",
			Description: "Two Deployments roll out the same release; " +
				"one image tag was never pushed and its pod fails with " +
				"ErrImagePull while the other rollout succeeds.",
			Root: "deployment/storefront/backend", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"registry//" + clusterRegistry,
				"deployment/storefront/frontend", "node//n1", "node//n2"},
		},
		build: buildHeldoutImageTag,
	}
}

func buildHeldoutImageTag(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	frontend := c.deployment("storefront", "frontend",
		clusterRegistry+"/storefront/frontend:2.3.1", 2)
	backend := c.deployment("storefront", "backend",
		clusterRegistry+"/storefront/backend:2.3.1", 2)
	for _, w := range []*workload{frontend, backend} {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(2 * time.Minute)
	frontendOld := []*corev1.Pod{last(c, frontend.pod(0, "")),
		last(c, frontend.pod(1, ""))}
	for _, w := range []*workload{frontend, backend} {
		image := w.deployment.Spec.Template.Spec.Containers[0].Image
		rs := w.rollout(func(spec *corev1.PodSpec) {
			spec.Containers[0].Image = image[:len(image)-5] + "2.4.0"
		})
		w.setReady(2)
		c.update(w.deployment)
		c.create(rs)
	}
	c.create(frontend.pod(0, "n1", startedNow, notReady),
		frontend.pod(1, "n2", startedNow, notReady))
	c.after(20 * time.Second)
	c.update(frontend.pod(0, "n1", startedNow),
		frontend.pod(1, "n2", startedNow))
	c.update(frontend.objects())
	c.remove(frontendOld[0], frontendOld[1])
	image := backend.deployment.Spec.Template.Spec.Containers[0].Image
	for n := range 6 {
		clusterPullFailure(c, backend, 0, "n1", n, image+": not found")
		c.after(40 * time.Second)
	}
}

// heldoutSidecarCrash: a rollout bumps the log shipper sidecar (a plain
// second container, not a native sidecar) to a major version that no
// longer accepts the mounted configuration. The shipper crash-loops;
// the app container keeps running but the new pod is never ready, and
// the previous revision keeps serving. The rollout is the root.
func heldoutSidecarCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-sidecar-crash",
			Description: "A rollout upgrades a log-shipper container " +
				"that runs beside the app; it crash-loops on its old " +
				"configuration while the app container keeps running.",
			Root: "deployment/orders/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: buildHeldoutSidecar,
	}
}

func buildHeldoutSidecar(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("orders", "api", "registry.example.com/orders:6.0", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers = append(spec.Containers, corev1.Container{
			Name: "log-shipper", Image: "fluent/fluent-bit:2.2.2",
		})
	})
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(2 * time.Minute)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[1].Image = "fluent/fluent-bit:3.0.0"
	})
	w.setReady(2)
	c.update(w.deployment)
	c.create(rs)
	c.create(w.pod(0, "n1", startedNow, notReady))
	c.after(30 * time.Second)
	message := "[error] [config] section 'output' tried to instance a " +
		"plugin name that doesn't exist: es_legacy"
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.update(w.pod(0, "n1", startedNow,
			shipperCrashLoop(message, restarts)))
		c.after(50 * time.Second)
	}
}

// shipperCrashLoop is a pod whose second container keeps exiting while
// the first one runs: the pod is not ready.
func shipperCrashLoop(message string, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		crashLoop(1, "Error", message, restarts)(c, pod)
		app, shipper := pod.Status.ContainerStatuses[1],
			pod.Status.ContainerStatuses[0]
		app.Name, app.Image = pod.Spec.Containers[0].Name,
			pod.Spec.Containers[0].Image
		shipper.Name, shipper.Image = pod.Spec.Containers[1].Name,
			pod.Spec.Containers[1].Image
		app.Ready, app.Started = true, boolPtr(true)
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{app, shipper}
	}
}
