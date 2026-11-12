package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// releaseCheckScenarios are releases that look like the cause until the
// evidence is checked: the version before them fails the same way, or
// the failure is a replica count no release set.
func releaseCheckScenarios() []scenario {
	return []scenario{releasePreviousVersionFails(),
		scaledToZeroThenReleased()}
}

// ordersStartLine is the line both versions of orders stop with.
const ordersStartLine = "Unhandled exception. " +
	"System.InvalidOperationException: persistence requires " +
	"connection string 'OrdersDb'."

// releasePreviousVersionFails: orders, which a GitOps controller
// deploys, gets a new image whose pod stops at start on a missing
// connection string; the pod of the previous version, restarted, stops
// with the same line from the same minute. The release is not what broke
// it, and reverting it would bring back a version that fails as well.
func releasePreviousVersionFails() scenario {
	return scenario{
		expect: expectation{
			Name: "release-previous-version-fails",
			Description: "A release's pod stops at start on a missing " +
				"connection string, and the restarted pod of the " +
				"previous version stops with the same line.",
			Root: "deployment/shop/orders", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: buildReleasePreviousFails,
	}
}

func buildReleasePreviousFails(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "orders", "registry.example.com/orders:1.0",
		1)
	w.deployment.Labels = mergeLabels(w.deployment.Labels,
		map[string]string{"argocd.argoproj.io/instance": "orders"})
	c.list(w.objects())
	old := w.pod(0, "n1")
	c.list(old)
	c.after(10 * time.Minute)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/orders:1.1"
	})
	w.deployment.ManagedFields = []metav1.ManagedFieldsEntry{{
		Manager: "argocd-controller", Time: &metav1.Time{Time: c.now},
		Operation: metav1.ManagedFieldsOperationUpdate,
	}}
	c.update(w.deployment)
	c.create(rs)
	c.create(w.pod(0, "n2", startedNow, notReady))
	for restarts := int32(1); restarts <= 6; restarts++ {
		c.after(time.Minute)
		c.update(w.pod(0, "n2", startedNow, crashLoop(1, "Error",
			ordersStartLine, restarts)))
		old = crashAgain(c, old, restarts+2)
	}
	w.setReady(0)
	c.update(w.deployment)
	c.after(5 * time.Minute)
}

// crashAgain records one more crash of a pod built earlier, whatever
// template its Deployment has now.
func crashAgain(c *cluster, pod *corev1.Pod, restarts int32) *corev1.Pod {
	next := pod.DeepCopy()
	crashLoop(1, "Error", ordersStartLine, restarts)(c, next)
	c.update(next)
	return next
}

// scaledToZeroThenReleased: the shop API is scaled to zero while an
// Ingress still routes to its Service; half a minute later a GitOps
// controller syncs a new image into the Deployment, still at zero. The
// replica count, not the new image, is what leaves the route without
// backends, so the release is not blamed.
func scaledToZeroThenReleased() scenario {
	return scenario{
		expect: expectation{
			Name: "scaled-to-zero-then-released",
			Description: "A Deployment at zero replicas that an Ingress " +
				"routes to gets a new image while it stays at zero.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			buildScaledToZero(c, true, false)
			c.after(30 * time.Second)
			w := c.deployment("shop", "api", "registry.example.com/api:1.0",
				0)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/api:1.1"
			})
			w.deployment.ManagedFields = []metav1.ManagedFieldsEntry{{
				Manager: "argocd-controller", Time: &metav1.Time{Time: c.now},
				Operation: metav1.ManagedFieldsOperationUpdate,
			}}
			c.update(w.deployment)
			c.create(rs)
			c.after(10 * time.Minute)
		},
	}
}
