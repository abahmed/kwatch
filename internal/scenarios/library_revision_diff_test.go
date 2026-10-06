package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// revisionDiffScenarios are bad releases where the message names what
// the new revision changed from the healthy one.
func revisionDiffScenarios() []scenario {
	return []scenario{
		badConfigValueChange(), memoryLimitLoweredRevision(),
		imageAndEnvChangeRanking(),
	}
}

// numberedRollout rolls the workload to its next revision, as a person
// edits the Deployment: the Deployment is updated by editor, the new
// ReplicaSet carries its revision number, and both are delivered.
func numberedRollout(
	c *cluster, w *workload, editor, revision string,
	mutate func(*corev1.PodSpec),
) {
	rs := w.rollout(mutate)
	rs.Annotations = map[string]string{revisionAnnotation: revision}
	editedBy(c, w.deployment, editor)
	c.update(w.deployment)
	c.create(rs)
}

// revisionBase lists a healthy revision 13 of checkout with two pods.
func revisionBase(
	c *cluster, mount string, tune func(*corev1.Container),
) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "checkout",
		"registry.example.com/checkout:5.1", 2)
	w.replicaSet.Annotations = map[string]string{revisionAnnotation: "13"}
	spec := &w.deployment.Spec.Template.Spec
	if tune != nil {
		tune(&spec.Containers[0])
	}
	if mount != "" {
		spec.Volumes = []corev1.Volume{{Name: "config",
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.
				ConfigMapVolumeSource{LocalObjectReference: corev1.
				LocalObjectReference{Name: mount}}}}}
	}
	w.replicaSet = w.newReplicaSet()
	w.replicaSet.Annotations = map[string]string{revisionAnnotation: "13"}
	w.setReady(2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	return w
}

// badConfigValueChange: an env var is pointed at a host that does not
// exist. The only template change of revision 14 is that value.
func badConfigValueChange() scenario {
	return scenario{
		expect: expectation{
			Name: "bad-config-value-change",
			Description: "A Deployment's DB_HOST env var is changed to a " +
				"host that does not exist; the pods of the new " +
				"revision crash-loop and the previous one stays healthy.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := revisionBase(c, "", func(c *corev1.Container) {
				c.Env = []corev1.EnvVar{{Name: "DB_HOST", Value: "db-old"}}
			})
			c.after(5 * time.Minute)
			numberedRollout(c, w, "alice", "14", func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{
					{Name: "DB_HOST", Value: "db-new"}}
			})
			c.create(w.pod(0, "n1", startedNow, notReady))
			c.after(40 * time.Second)
			for restarts := int32(2); restarts <= 5; restarts += 3 {
				c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
					"dial tcp: lookup db-new: no such host", restarts)))
				c.after(2 * time.Minute)
			}
		},
	}
}

// memoryLimitLoweredRevision: the memory limit is halved and the new
// pods are killed for it.
func memoryLimitLoweredRevision() scenario {
	return scenario{
		expect: expectation{
			Name: "memory-limit-lowered-revision",
			Description: "A Deployment's memory limit is lowered from " +
				"512Mi to 256Mi; the pods of the new revision are " +
				"OOMKilled and the previous revision stays healthy.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			limit := func(q string) corev1.ResourceRequirements {
				return corev1.ResourceRequirements{Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse(q)}}
			}
			w := revisionBase(c, "", func(c *corev1.Container) {
				c.Resources = limit("512Mi")
			})
			c.after(5 * time.Minute)
			numberedRollout(c, w, "alice", "14", func(spec *corev1.PodSpec) {
				spec.Containers[0].Resources = limit("256Mi")
			})
			c.create(w.pod(0, "n1", startedNow, notReady))
			c.after(40 * time.Second)
			for restarts := int32(2); restarts <= 5; restarts += 3 {
				c.update(w.pod(0, "n1", startedNow,
					crashLoop(137, "OOMKilled", "", restarts)))
				c.after(2 * time.Minute)
			}
		},
	}
}

// imageAndEnvChangeRanking: one release changes the image, an env var and
// a probe at once, and an edit to the ConfigMap it mounts came just
// before. An unrelated ConfigMap edit is not named.
func imageAndEnvChangeRanking() scenario {
	return scenario{
		expect: expectation{
			Name: "image-and-env-change-ranking",
			Description: "A release changes the image, an env var and a " +
				"probe together; the message ranks the image first and " +
				"names the edit of the ConfigMap the pods mount, not an " +
				"unrelated one.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1", "node//n2",
				"configmap/shop/unrelated-flags"},
		},
		build: func(c *cluster) {
			mounted := configMap(c, "shop", "checkout-config",
				map[string]string{"limit": "10"})
			unrelated := configMap(c, "shop", "unrelated-flags",
				map[string]string{"banner": "off"})
			c.list(mounted, unrelated)
			w := revisionBase(c, "checkout-config", nil)
			c.after(2 * time.Minute)
			edit := last(c, mounted)
			edit.Data["limit"] = "20"
			editedBy(c, edit, "bob")
			c.update(edit)
			other := last(c, unrelated)
			other.Data["banner"] = "on"
			editedBy(c, other, "eve")
			c.update(other)
			c.after(2 * time.Minute)
			numberedRollout(c, w, "alice", "14", func(spec *corev1.PodSpec) {
				container := &spec.Containers[0]
				container.Image = "registry.example.com/checkout:5.2"
				container.Env = []corev1.EnvVar{
					{Name: "LOG_LEVEL", Value: "debug"}}
				container.ReadinessProbe = &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.
						HTTPGetAction{Path: "/ready", Port: intstr.FromInt(8080)}},
					PeriodSeconds: 5, TimeoutSeconds: 1, FailureThreshold: 3}
			})
			c.create(w.pod(0, "n1", startedNow, notReady))
			c.after(40 * time.Second)
			for restarts := int32(2); restarts <= 5; restarts += 3 {
				c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
					"panic: invalid probe configuration", restarts)))
				c.after(2 * time.Minute)
			}
		},
	}
}
