package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// movedRootReturns: a Deployment reads its environment from a ConfigMap
// that does not exist, so its pods cannot start. Creating the ConfigMap
// fixes it; while the incident recovers the cause may be revised from the
// ConfigMap to the Deployment. Twenty minutes after the resolve the
// ConfigMap is deleted again and the pods are replaced: the same failure
// is back, and it must find its earlier incident whatever root that
// incident ended with ("failing again", not a new story with no
// history).
func movedRootReturns() scenario {
	return scenario{
		expect: expectation{
			Name: "moved-root-returns",
			Description: "A missing ConfigMap stops a Deployment's pods; " +
				"it is created, the incident resolves, and twenty " +
				"minutes later the ConfigMap is deleted again and the " +
				"pods are replaced.",
			Root: "configmap/shop/settings", Tier: "notify",
			MaxMessages: 4, RevisesRoot: true,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(10 * time.Minute),
		},
		build: buildMovedRootReturns,
	}
}

func buildMovedRootReturns(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "refailure", "registry.example.com/app:4", 2)
	configTemplate(w, advEnvFromConfigMap("settings"))
	c.list(w.deployment, w.replicaSet)
	w.setReady(0)
	c.update(w.deployment, w.replicaSet)
	movedRootBreak(c, w, 0)
	c.after(6 * time.Minute)
	cm := configMap(c, "shop", "settings", map[string]string{"MODE": "on"})
	c.create(cm)
	c.after(20 * time.Second)
	c.update(w.pod(0, "n1", startedNow), w.pod(1, "n2", startedNow))
	w.setReady(2)
	c.update(w.objects())
	c.after(30 * time.Minute)
	c.remove(cm)
	c.after(10 * time.Second)
	for i := 0; i < 2; i++ {
		c.remove(w.pod(i, []string{"n1", "n2"}[i]))
	}
	w.setReady(0)
	c.update(w.objects())
	movedRootBreak(c, w, 2)
}

// movedRootBreak creates two pods, from index from on, that cannot start
// because the ConfigMap they read is missing.
func movedRootBreak(c *cluster, w *workload, from int) {
	message := "configmap \"settings\" not found"
	var stuck []*corev1.Pod
	for i, node := range []string{"n1", "n2"} {
		stuck = append(stuck, w.pod(from+i, node, startedNow,
			waiting("CreateContainerConfigError", message)))
	}
	c.create(stuck[0], stuck[1])
	configWarn(c, w, from, from+2, "Failed", "Error: "+message)
}
