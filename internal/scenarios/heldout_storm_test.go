package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// heldoutStormScenarios are held-out storms: several independent
// failures at once. See heldoutLibrary for the rule they follow.
func heldoutStormScenarios() []scenario {
	return []scenario{heldoutMixedStorm()}
}

// heldoutMixedStorm: within three minutes of a Friday deploy window,
// three unrelated things break. Node n4 stops reporting and the six
// pods on it turn not ready; a new payout Deployment references a Secret
// nobody created; and the indexer rolls out an image tag with a typo.
// Each is its own incident. The lost node pages, so it is heard first.
func heldoutMixedStorm() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-mixed-storm",
			Description: "A node is lost, a new Deployment misses its " +
				"Secret and another rollout has a mistyped image tag, " +
				"all within three minutes.",
			Root: "node//n4", Tier: "page",
			OtherRoots: []string{"secret/finance/payout-keys",
				"deployment/search/indexer"},
			MaxMessages: 8,
			MustNotBlame: []string{"zone//zone-c",
				"registry//" + clusterRegistry, "node//n1", "node//n2"},
		},
		build: buildHeldoutMixedStorm,
	}
}

func buildHeldoutMixedStorm(c *cluster) {
	nodes := []string{"n1", "n2", "n3", "n4"}
	for _, name := range nodes {
		c.list(c.node(name, "zone-c"))
	}
	stranded := nodeFleet(c, "shop", []string{"cart", "orders", "promo",
		"reviews", "search-ui", "wishlist"}, []string{"n3", "n4"})
	indexer := c.deployment("search", "indexer",
		clusterRegistry+"/search/indexer:4.2.0", 2)
	c.list(indexer.objects())
	c.list(indexer.pod(0, "n1"), indexer.pod(1, "n2"))
	c.after(2 * time.Minute)
	heldoutStormNodeLoss(c, stranded)
	c.after(50 * time.Second)
	heldoutStormMissingSecret(c)
	c.after(40 * time.Second)
	image := clusterRegistry + "/search/indexer:4.2.l"
	rs := indexer.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = image
	})
	indexer.setReady(2)
	c.update(indexer.deployment)
	c.create(rs)
	for n := range 6 {
		clusterPullFailure(c, indexer, 0, "n1", n, image+": not found")
		c.after(40 * time.Second)
	}
}

// heldoutStormNodeLoss loses n4: the node turns Unknown, then every pod
// on it is marked not ready.
func heldoutStormNodeLoss(c *cluster, stranded []*workload) {
	nodeLose(c, last(c, c.node("n4", "zone-c")))
	c.after(40 * time.Second)
	for _, w := range stranded {
		c.update(w.pod(1, "n4", notReady))
		w.setReady(1)
		c.update(w.objects())
	}
}

// heldoutStormMissingSecret creates the payout Deployment whose pods
// read a Secret that does not exist.
func heldoutStormMissingSecret(c *cluster) {
	w := c.deployment("finance", "payout",
		"registry.example.com/payout:1.0", 2)
	name := c.n("payout-keys")
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
			},
		}}
	})
	w.setReady(0)
	c.create(w.objects())
	message := fmt.Sprintf("secret %q not found", name)
	for i := range 2 {
		c.create(w.pod(i, "n"+itoa(i+1), startedNow,
			waiting("CreateContainerConfigError", message)))
	}
	configWarn(c, w, 0, 2, "Failed", "Error: "+message)
}
