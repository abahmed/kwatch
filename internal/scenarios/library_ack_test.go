package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ackScenarios are incidents someone says they are looking at, and
// incidents that belong to a team.
func ackScenarios() []scenario {
	return []scenario{
		ackAnnotation(), ackAtAnnounce(), ownerRouted(),
	}
}

// crashingRollout lists a payments Deployment and rolls out an image
// whose pods crash-loop, as badRollout does. The Deployment starts with
// the annotations given.
func crashingRollout(c *cluster, annotations map[string]string) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "payments",
		"registry.example.com/payments:2.2", 2)
	w.deployment.Annotations = annotations
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(70 * time.Second)
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/payments:2.3"
	})
	c.update(w.deployment)
	c.create(rs)
	c.create(w.pod(0, "n1", startedNow, notReady))
	c.after(40 * time.Second)
	c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
		"panic: missing key DB_PASSWORD_V2", 3)))
	c.after(2 * time.Minute)
	c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
		"panic: missing key DB_PASSWORD_V2", 5)))
	return w
}

// annotate sets or, with an empty value and remove, drops an annotation
// of the Deployment, as kubectl annotate does.
func annotate(c *cluster, w *workload, key, value string, remove bool) {
	annotations := map[string]string{}
	for k, v := range w.deployment.Annotations {
		annotations[k] = v
	}
	if remove {
		delete(annotations, key)
	} else {
		annotations[key] = value
	}
	w.deployment.Annotations = annotations
	c.update(w.deployment)
}

// ackAnnotation: someone annotates the failing Deployment with
// kwatch.io/ack. The thread hears it once, no reminder follows in the
// week the incident stays open, and removing the annotation is told once.
func ackAnnotation() scenario {
	return scenario{
		expect: expectation{
			Name: "ack-annotation",
			Description: "A crash-looping Deployment is acknowledged " +
				"with an annotation; it stays quiet for eight days, " +
				"and the removal of the annotation is told.",
			Root: "deployment/shop/payments", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//n1"},
			Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			w := crashingRollout(c, nil)
			c.after(5 * time.Minute)
			annotate(c, w, kube.AckAnnotation, "looking into it", false)
			c.after(8 * 24 * time.Hour)
			annotate(c, w, kube.AckAnnotation, "", true)
		},
	}
}

// ackAtAnnounce: the annotation is already on the Deployment when the
// new incident is announced. The announcement says so.
func ackAtAnnounce() scenario {
	return scenario{
		expect: expectation{
			Name: "ack-at-announce",
			Description: "A crash-looping Deployment still carries an " +
				"ack annotation when its incident is announced.",
			Root: "deployment/shop/payments", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
			Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			crashingRollout(c, map[string]string{
				kube.AckAnnotation: "left over from yesterday"})
		},
	}
}

// ownerRouted: the Deployment has no owner of its own but its namespace
// is labelled kwatch.io/owner, so the incident is routed to that team.
func ownerRouted() scenario {
	return scenario{
		expect: expectation{
			Name: "owner-routed",
			Description: "A crash-looping Deployment in a namespace " +
				"labelled with its owner.",
			Root: "deployment/shop/payments", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			namespace := &corev1.Namespace{ObjectMeta: c.meta("", "shop")}
			namespace.Labels = map[string]string{kube.OwnerKey: "payments"}
			c.list(namespace)
			crashingRollout(c, nil)
		},
	}
}
