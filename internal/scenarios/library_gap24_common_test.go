package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// The gap24 library holds the scenarios written from SCENARIO_GAPS_24.md:
// ordinary faults whose first message is judged on its words. The terms
// each message must and must not contain live in wording_terms_test.go.

// g24Exp is a label with a budget of n messages and a ten-minute tail.
func g24Exp(
	name, desc, root, tier string, n int, not ...string,
) expectation {
	return g2Exp(name, desc, root, tier, not...).budget(n).
		tail(10 * time.Minute)
}

// g24Roll lists a healthy Deployment of n pods on two nodes, then rolls
// it out with after; before edits the first spec when it is not nil.
func g24Roll(
	c *cluster, namespace, name, image string, n int32,
	before, after func(*corev1.PodSpec),
) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment(namespace, name, image, n)
	if before != nil {
		configTemplate(w, before)
	}
	c.list(w.objects())
	for i := range int(n) {
		c.list(w.pod(i, "n1"))
	}
	c.after(2 * time.Minute)
	rs := w.rollout(after)
	d := w.deployment
	d.Status.Replicas, d.Status.UpdatedReplicas = n+1, 1
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = n, n
	d.Status.UnavailableReplicas = 1
	c.update(d)
	c.create(rs)
	return w
}

// g24Image sets the image of the first container.
func g24Image(image string) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) { spec.Containers[0].Image = image }
}
