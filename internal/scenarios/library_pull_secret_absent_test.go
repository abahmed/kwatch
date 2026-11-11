package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// pullSecretAbsentScenarios are pods that name an image pull Secret
// nobody created. The kubelet only warns and pulls without it, so the
// absence is a failure only when a pull is refused for want of a login
// (registry-login-missing-secret is that story).
func pullSecretAbsentScenarios() []scenario {
	return []scenario{
		pullSecretAbsentHealthy(), pullSecretAbsentSlowStart(),
		pullSecretAbsentOwnCrash(),
	}
}

// absentPullSecretNotice is the kubelet's event for a pull Secret it
// could not read.
const absentPullSecretNotice = "Unable to retrieve some image pull " +
	"secrets (regcred); attempting to pull the image may not succeed."

// reportsApp is a two-replica Deployment of a public image that names a
// pull Secret that does not exist.
func reportsApp(c *cluster) *workload {
	w := c.deployment("shop", "reports",
		"registry.example.com/reports:2.4", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{{
			Name: c.n("regcred"),
		}}
	})
	return w
}

// pullSecretNotices records the kubelet's notice for both replicas.
func pullSecretNotices(c *cluster, w *workload, count int32) {
	for i := range 2 {
		pod := last(c, w.pod(i, ""))
		c.warn(c.warningEvent(pod, "Pod",
			"FailedToRetrieveImagePullSecret", absentPullSecretNotice,
			"kubelet", count))
	}
}

// pullSecretAbsentHealthy: both pods run and are ready; the kubelet
// repeats its notice about the absent pull Secret for half an hour.
// Nothing fails, so nothing is said.
func pullSecretAbsentHealthy() scenario {
	return scenario{
		expect: expectation{
			Name: "pull-secret-absent-pods-healthy",
			Description: "Healthy pods name an image pull Secret that " +
				"does not exist; the kubelet keeps warning that it " +
				"could not retrieve it.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := reportsApp(c)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			for n := range 10 {
				c.after(3 * time.Minute)
				pullSecretNotices(c, w, int32(n+1))
			}
		},
	}
}

// pullSecretAbsentSlowStart: a new Deployment's pods spend 75 seconds
// creating their containers, then run. The absent pull Secret did not
// slow them down and broke nothing.
func pullSecretAbsentSlowStart() scenario {
	return scenario{
		expect: expectation{
			Name: "pull-secret-absent-slow-start",
			Description: "New pods that name an absent image pull " +
				"Secret take 75 seconds to create their containers " +
				"and then run.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.after(time.Minute)
			w := reportsApp(c)
			w.setReady(0)
			c.create(w.objects())
			for i := range 2 {
				c.create(w.pod(i, "n"+itoa(i+1), startedNow, creating))
			}
			pullSecretNotices(c, w, 1)
			c.after(75 * time.Second)
			for i := range 2 {
				c.update(w.pod(i, "n"+itoa(i+1), startedNow))
			}
			w.setReady(2)
			c.update(w.objects())
			c.after(5 * time.Minute)
		},
	}
}

// pullSecretAbsentOwnCrash: the pods pulled their image without the
// absent pull Secret and crash-loop on a panic of their own. The
// workload is the root; the Secret has nothing to do with it.
func pullSecretAbsentOwnCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "pull-secret-absent-own-crash",
			Description: "Pods that name an absent image pull Secret " +
				"pulled their image and crash-loop on their own panic.",
			Root: "deployment/shop/reports", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"secret/shop/regcred", "node//n1",
				"node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := reportsApp(c)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(2 * time.Minute)
			crash := "panic: template: report.tmpl:14: unexpected EOF"
			for _, restarts := range []int32{2, 4, 6} {
				for i := range 2 {
					c.update(w.pod(i, "n"+itoa(i+1),
						crashLoop(2, "Error", crash, restarts)))
				}
				w.setReady(0)
				c.update(w.deployment, w.replicaSet)
				pullSecretNotices(c, w, restarts)
				c.after(2 * time.Minute)
			}
		},
	}
}
