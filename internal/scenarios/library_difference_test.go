package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	tagRef    = "registry.example.com/api:latest"
	oldDigest = "registry.example.com/api@sha256:" +
		"aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff66667777888899990000"
	newDigest = "registry.example.com/api@sha256:" +
		"9f1c0a2b44d1eeee5555ffff66667777888899990000aaaa1111bbbb22223333"
)

// differenceScenarios are failures where one attribute cleanly
// separates the failing replicas from the healthy ones.
func differenceScenarios() []scenario {
	return []scenario{sameTagNewDigest(), sameTagMixedDigestsHealthy(),
		zoneSeparatesReplicas()}
}

// runsDigest sets the image ID the kubelet reports for the pod's app
// container: the build the tag resolved to.
func runsDigest(imageID string) podState {
	return func(_ *cluster, pod *corev1.Pod) {
		for i := range pod.Status.ContainerStatuses {
			pod.Status.ContainerStatuses[i].ImageID = imageID
		}
	}
}

// driftingTag lists a Deployment of four replicas of one mutable tag on
// two nodes. The first two run the old build; the others were started
// later and run the newer one. It returns the pods in order.
func driftingTag(c *cluster) (*workload, []*corev1.Pod) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "api", tagRef, 4)
	c.list(w.objects())
	pods := []*corev1.Pod{
		w.pod(0, "n1", runsDigest(oldDigest)),
		w.pod(1, "n2", runsDigest(oldDigest)),
		w.pod(2, "n1", runsDigest(oldDigest)),
		w.pod(3, "n2", runsDigest(oldDigest)),
	}
	c.list(pods[0], pods[1], pods[2], pods[3])
	c.after(time.Hour)
	return w, pods
}

// sameTagNewDigest: the tag api:latest moved. Two replicas restarted
// and pulled the new build, which crash-loops; the other two keep
// running the old build. The digest is the difference.
func sameTagNewDigest() scenario {
	return scenario{
		expect: expectation{
			Name: "same-tag-new-digest",
			Description: "Two of four replicas of one mutable tag pulled " +
				"a new build that crash-loops; the other two run the " +
				"old digest of the same tag.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1", "node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) {
			w, _ := driftingTag(c)
			// The new build runs fine for a while before it breaks.
			c.update(w.pod(0, "n1", runsDigest(newDigest)),
				w.pod(1, "n2", runsDigest(newDigest)))
			c.after(8 * time.Minute)
			crash := "panic: unsupported schema version 7"
			for _, restarts := range []int32{3, 5} {
				for _, i := range []int{0, 1} {
					c.update(w.pod(i, "n"+itoa(i+1), runsDigest(newDigest),
						crashLoop(1, "Error", crash, restarts)))
				}
				w.setReady(2)
				c.update(w.deployment, w.replicaSet)
				c.after(2 * time.Minute)
			}
		},
	}
}

// sameTagMixedDigestsHealthy: the same drift, but every replica is
// healthy. Mixed digests are a risk to digest, never a page.
func sameTagMixedDigestsHealthy() scenario {
	return scenario{
		expect: expectation{
			Name: "same-tag-mixed-digests-healthy",
			Description: "Replicas of one mutable tag run two digests " +
				"and all are healthy: a configuration risk, not a page.",
			Quiet: true,
		},
		build: func(c *cluster) {
			w, _ := driftingTag(c)
			c.update(w.pod(0, "n1", runsDigest(newDigest)),
				w.pod(1, "n2", runsDigest(newDigest)))
			c.after(30 * time.Minute)
		},
	}
}

// zoneSeparatesReplicas: a release changes a setting of a workload with
// four replicas on four nodes, two in each zone. After it the two in
// zone-b crash-loop and the two in zone-a are healthy. Replicas share
// one spec, so the zone is what separates them, and the message says so.
func zoneSeparatesReplicas() scenario {
	return scenario{
		expect: expectation{
			Name: "zone-separates-replicas",
			Description: "A release is followed by two of four replicas " +
				"crash-looping; both run in zone-b, the healthy two in " +
				"zone-a.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//a1", "node//a2"},
		},
		build: func(c *cluster) {
			c.list(c.node("a1", "zone-a"), c.node("a2", "zone-a"),
				c.node("b1", "zone-b"), c.node("b2", "zone-b"))
			w := c.deployment("shop", "api",
				"registry.example.com/api:3.4", 4)
			c.list(w.objects())
			nodes := []string{"b1", "b2", "a1", "a2"}
			var old []*corev1.Pod
			for i, node := range nodes {
				old = append(old, w.pod(i, node))
			}
			c.list(old[0], old[1], old[2], old[3])
			c.after(time.Hour)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{{
					Name: "LICENSE_URL", Value: "https://licensing.internal"}}
			})
			c.update(w.deployment)
			c.create(rs)
			c.remove(old[0], old[1], old[2], old[3])
			for i, node := range nodes {
				c.create(w.pod(i, node, startedNow))
			}
			c.after(40 * time.Second)
			crash := "fatal: cannot load license from licensing.internal"
			for _, restarts := range []int32{3, 5} {
				for i, node := range nodes[:2] {
					c.update(w.pod(i, node, startedNow,
						crashLoop(1, "Error", crash, restarts)))
				}
				w.setReady(2)
				c.update(w.deployment, w.replicaSet)
				c.after(2 * time.Minute)
			}
		},
	}
}
