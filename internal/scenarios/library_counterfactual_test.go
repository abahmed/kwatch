package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// counterfactualScenarios are failures where a healthy twin elsewhere
// (the same image, the other replicas) rules a cause in or out.
func counterfactualScenarios() []scenario {
	return append([]scenario{
		configBreaksWhileImageRunsElsewhere(), nodeLocalWithHealthyReplicas(),
		badImageEverywhere(),
	}, differenceScenarios()...)
}

// configBreaksWhileImageRunsElsewhere: a ConfigMap edit breaks one
// workload while the very same image runs fine in another namespace.
// The image is not the difference, so the ConfigMap is the cause.
func configBreaksWhileImageRunsElsewhere() scenario {
	return scenario{
		expect: expectation{
			Name: "config-breaks-image-runs-elsewhere",
			Description: "A ConfigMap edit crash-loops one workload; the " +
				"same image runs healthy in another namespace.",
			Root: "configmap/web/frontend-config", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/web/frontend", "node//n1",
				"node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) {
			const image = "registry.example.com/frontend:7.2"
			cm := configMap(c, "web", "frontend-config",
				map[string]string{"app.yaml": "max_connections: 100"})
			w := c.deployment("web", "frontend", image, 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cm.Name,
							},
						},
					},
				}}
			})
			twin := c.deployment("staging-2", "frontend", image, 2)
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), cm)
			c.list(w.objects())
			c.list(twin.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.list(twin.pod(0, "n1"), twin.pod(1, "n2"))
			c.after(90 * time.Second)
			edited := last(c, cm)
			edited.Data["app.yaml"] = "max_connections: unlimited"
			c.update(edited)
			c.after(50 * time.Second)
			crash := "config: /etc/frontend/app.yaml: max_connections: " +
				"cannot unmarshal !!str `unlimited` into int"
			for _, restarts := range []int32{2, 4} {
				for i := range 2 {
					c.update(w.pod(i, "n"+itoa(i+1),
						crashLoop(1, "Error", crash, restarts)))
				}
				w.setReady(0)
				c.update(w.deployment, w.replicaSet)
				c.after(2 * time.Minute)
			}
		},
	}
}

// nodeLocalWithHealthyReplicas: n1 stops reporting. Two of the three
// replicas of one workload ran there and go not ready; the third runs
// healthy on n2, as do the other workloads' pods. The node is the cause.
func nodeLocalWithHealthyReplicas() scenario {
	return scenario{
		expect: expectation{
			Name: "node-local-healthy-replicas",
			Description: "A node stops reporting; two replicas of an app " +
				"on it go not ready while its third replica and a " +
				"neighbour workload stay healthy on other nodes.",
			Root: "node//n1", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api", "zone//zone-a"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			api := c.deployment("shop", "api",
				"registry.example.com/api:3.4", 3)
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:1.9", 2)
			c.list(api.objects())
			c.list(cart.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n1"), api.pod(2, "n2"))
			c.list(cart.pod(0, "n2"), cart.pod(1, "n3"))
			c.after(time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Second)
			c.update(api.pod(0, "n1", notReady), api.pod(1, "n1", notReady))
			api.setReady(1)
			c.update(api.deployment, api.replicaSet)
		},
	}
}

// badImageEverywhere: a release with a bad image replaces every replica
// of one workload, on three nodes, and all of them crash-loop; a
// neighbour on other images stays healthy. The release is the cause.
func badImageEverywhere() scenario {
	return scenario{
		expect: expectation{
			Name: "bad-image-everywhere",
			Description: "A release's new image crash-loops on every " +
				"replica of a workload across three nodes; another " +
				"workload on the same nodes is healthy.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1", "node//n2",
				"node//n3", "zone//zone-a"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "checkout",
				"registry.example.com/checkout:2.0", 3)
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:1.9", 2)
			c.list(w.objects())
			c.list(cart.objects())
			old := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n2"),
				w.pod(2, "n3")}
			c.list(old[0], old[1], old[2])
			c.list(cart.pod(0, "n1"), cart.pod(1, "n2"))
			c.after(70 * time.Second)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image =
					"registry.example.com/checkout:2.1"
			})
			c.update(w.deployment)
			c.create(rs)
			c.remove(old[0], old[1], old[2])
			crash := "panic: unsupported schema version 7"
			for i := range 3 {
				c.create(w.pod(i, "n"+itoa(i+1), startedNow, notReady))
			}
			c.after(40 * time.Second)
			for _, restarts := range []int32{3, 5} {
				for i := range 3 {
					c.update(w.pod(i, "n"+itoa(i+1), startedNow,
						crashLoop(1, "Error", crash, restarts)))
				}
				c.after(2 * time.Minute)
			}
		},
	}
}
