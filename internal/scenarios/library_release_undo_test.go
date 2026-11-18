package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// releaseUndoScenarios are bad releases where kwatch must not suggest
// the rollback it would offer for a clear one: the release is only a
// possible cause, or the revision a rollback returns to never served.
func releaseUndoScenarios() []scenario {
	return []scenario{releaseBlamedPossible(), releasePreviousNeverReady()}
}

// releaseUndoCrash is what the new pods of these releases stop with: an
// error that names no change.
const releaseUndoCrash = "fatal: initialization failed (exit 1)"

// releaseBlamedPossible: the Secret checkout reads is rotated and, a
// few seconds later, revision 14 of checkout rolls out a new image. Its
// new pod crash-loops with an error that names neither; revision 13
// keeps serving. The release might be the cause, or the Secret: kwatch
// offers the read-only checks and what would confirm the release, not
// a rollback.
func releaseBlamedPossible() scenario {
	return scenario{
		expect: expectation{
			Name: "release-blamed-possible",
			Description: "A Secret rotation and a release land together; " +
				"the new pod crash-loops with an error naming neither " +
				"while the previous revision serves.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages: 2, Ambiguous: true,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			secret := configSecret(c, "shop", "checkout-keys",
				"signing-key")
			w := revisionBase(c, "", func(ct *corev1.Container) {
				ct.Env = []corev1.EnvVar{configSecretEnv("SIGNING_KEY",
					secret.Name, "signing-key")}
			})
			c.list(secret)
			c.after(5 * time.Minute)
			rotated := last(c, secret)
			rotated.Data["signing-key"] = []byte("rotated-key")
			c.update(rotated)
			c.after(10 * time.Second)
			numberedRollout(c, w, "alice", "14", func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/checkout:5.2"
			})
			releaseUndoCrashLoop(c, w)
		},
	}
}

// releasePreviousNeverReady: checkout has been kept at zero replicas
// since before kwatch started, at revision 13, so kwatch never saw
// revision 13 run. It is scaled up with revision 14, a new image, and
// the new pods crash-loop. A rollback goes to revision 13, which kwatch
// has no record of running, so kwatch says so instead of suggesting
// it, and keeps the read-only checks.
func releasePreviousNeverReady() scenario {
	return scenario{
		expect: expectation{
			Name: "release-previous-never-ready",
			Description: "A Deployment kept at zero replicas is scaled up " +
				"with a release whose pods crash-loop; the revision " +
				"before it never ran while kwatch watched, so no " +
				"rollback to it is suggested.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "checkout",
				"registry.example.com/checkout:5.1", 0)
			w.replicaSet.Annotations = map[string]string{
				revisionAnnotation: "13"}
			c.list(w.objects())
			c.after(5 * time.Minute)
			previous := w.replicaSet
			setReplicas(w, 2)
			w.replicaSet = previous
			numberedRollout(c, w, "alice", "14", func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/checkout:5.2"
			})
			releaseUndoCrashLoop(c, w)
		},
	}
}

// releaseUndoCrashLoop starts the first pod of the workload's newest
// revision and lets it crash-loop for a while.
func releaseUndoCrashLoop(c *cluster, w *workload) {
	c.create(w.pod(0, "n1", startedNow, notReady))
	c.after(40 * time.Second)
	for restarts := int32(2); restarts <= 8; restarts += 3 {
		c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
			releaseUndoCrash, restarts)))
		c.after(2 * time.Minute)
	}
}
