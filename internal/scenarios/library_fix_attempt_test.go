package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// fixAttemptScenarios are incidents someone tries to fix while they are
// open: the thread says a rollout started, and later whether it worked.
func fixAttemptScenarios() []scenario {
	return []scenario{fixAttempt(true), fixAttempt(false)}
}

// fixAttempt: a bad rollout crash-loops payments. Alice rolls out
// revision 15 after the announcement. In the working variant its pods
// are healthy and the incident resolves; in the failing variant they
// crash too and the thread says so after ten minutes.
func fixAttempt(works bool) scenario {
	expect := expectation{
		Name: "fix-attempt-fails",
		Description: "A second rollout, made to fix a crash-looping " +
			"one, crash-loops as well; the thread says it still fails " +
			"ten minutes after the rollout.",
		Root: "deployment/shop/payments", Tier: "notify",
		MaxMessages: 5, MustNotBlame: []string{"node//n1", "node//n2"},
	}
	if works {
		expect.Name = "fix-attempt-works"
		expect.Description = "A second rollout, made to fix a " +
			"crash-looping one, brings the pods back; the thread says " +
			"the rollout started and the resolve credits it."
		expect.MaxMessages = 4
	}
	return scenario{expect: expect, build: func(c *cluster) {
		buildFixAttempt(c, works)
	}}
}

func buildFixAttempt(c *cluster, works bool) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "payments",
		"registry.example.com/payments:2.2", 2)
	c.list(w.objects())
	old := []runtime.Object{w.pod(0, "n1"), w.pod(1, "n2")}
	c.list(old...)
	c.after(70 * time.Second)
	bad := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/payments:2.3"
	})
	w.deployment.Annotations = map[string]string{revisionAnnotation: "14"}
	c.update(w.deployment)
	c.create(bad)
	c.remove(old...)
	crashing(c, w, 3, true)
	c.after(3 * time.Minute)
	crashing(c, w, 5, false)
	c.after(2 * time.Minute)
	badPods := []runtime.Object{w.pod(0, "n1"), w.pod(1, "n2")}

	// Alice's fix: revision 15.
	fix := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/payments:2.4"
	})
	w.deployment.Annotations = map[string]string{revisionAnnotation: "15"}
	editedBy(c, w.deployment, "alice")
	c.update(w.deployment)
	c.create(fix)
	if works {
		// The old pods keep failing until the new ones are ready.
		c.create(w.pod(0, "n1", startedNow, notReady),
			w.pod(1, "n2", startedNow, notReady))
		c.after(2 * time.Minute)
		c.remove(badPods...)
		c.update(w.pod(0, "n1"), w.pod(1, "n2"))
		c.after(8 * time.Minute)
		return
	}
	crashing(c, w, 1, true)
	c.after(time.Minute)
	c.remove(badPods...)
	for restarts := int32(2); restarts <= 7; restarts++ {
		crashing(c, w, restarts, false)
		c.after(2 * time.Minute)
	}
}

// crashing puts both pods of the workload's current revision into a
// crash loop; with first it creates them.
func crashing(c *cluster, w *workload, restarts int32, first bool) {
	update := c.update
	if first {
		update = c.create
	}
	for i, node := range []string{"n1", "n2"} {
		update(w.pod(i, node, startedNow, crashLoop(1, "Error",
			"panic: missing key DB_PASSWORD_V2", restarts)))
	}
}
