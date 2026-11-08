package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// blastOverlapScenarios are failures that follow two edits at once. The
// edit the failing pods share is likelier than the one few of them read.
func blastOverlapScenarios() []scenario {
	return []scenario{twoEditsOneSharedByMostPods()}
}

// configVolume mounts a ConfigMap into every container of the pod
// template.
func configVolume(name, configMap string) corev1.Volume {
	return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: configMap}}}}
}

// twoEditsOneSharedByMostPods: the orders Deployment (four replicas)
// reads app-defaults and orders-tuning; the batch Deployment (one
// replica) reads app-defaults. Both ConfigMaps are edited within twenty
// seconds and all five pods crash-loop with an error that names nothing.
// All five failing pods read app-defaults, four read orders-tuning: the
// shared edit is the likelier cause.
func twoEditsOneSharedByMostPods() scenario {
	return scenario{
		expect: expectation{
			Name: "change-blast-overlap",
			Description: "Two ConfigMaps are edited within twenty " +
				"seconds; every failing pod reads one and most read " +
				"the other.",
			Root: "configmap/shop/app-defaults", Tier: "notify",
			MaxMessages: 4,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/shop/batch", "node//n1"},
		},
		build: func(c *cluster) {
			defaults := configMap(c, "shop", "app-defaults",
				map[string]string{"app.yaml": "pool: 10"})
			tuning := configMap(c, "shop", "orders-tuning",
				map[string]string{"tuning.yaml": "batch: 50"})
			orders := c.deployment("shop", "orders",
				"registry.example.com/orders:5", 4)
			configTemplate(orders, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{
					configVolume("defaults", defaults.Name),
					configVolume("tuning", tuning.Name)}
			})
			batch := c.deployment("shop", "batch",
				"registry.example.com/batch:5", 1)
			configTemplate(batch, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{
					configVolume("defaults", defaults.Name)}
			})
			c.list(c.node("n1", "zone-a"), defaults, tuning)
			c.list(orders.objects())
			c.list(batch.objects())
			for i := range 4 {
				c.list(orders.pod(i, "n1"))
			}
			c.list(batch.pod(0, "n1"))
			c.after(90 * time.Second)
			editedDefaults := last(c, defaults)
			editedDefaults.Data["app.yaml"] = "pool: 0"
			c.update(editedDefaults)
			c.after(20 * time.Second)
			editedTuning := last(c, tuning)
			editedTuning.Data["tuning.yaml"] = "batch: 75"
			c.update(editedTuning)
			c.after(30 * time.Second)
			for _, restarts := range []int32{2, 4} {
				for i := range 4 {
					c.update(orders.pod(i, "n1",
						crashLoop(1, "Error", "exit status 1", restarts)))
				}
				c.update(batch.pod(0, "n1",
					crashLoop(1, "Error", "exit status 1", restarts)))
				orders.setReady(0)
				batch.setReady(0)
				c.update(orders.deployment, orders.replicaSet,
					batch.deployment, batch.replicaSet)
				c.after(2 * time.Minute)
			}
		},
	}
}
