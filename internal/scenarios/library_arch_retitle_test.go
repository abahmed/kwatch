package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// archRetitleLine is what the runtime prints when the binary is built
// for another CPU; the container leaves no termination message.
const archRetitleLine = "exec /usr/bin/sh: exec format error"

// execFormatAfterReleaseRetitled: a release moves the only replica of
// reports to an image built for amd64 only, on an arm64 node. The new
// pod exits at start with "exec format error", which only its previous
// log says, and some runs leave no line at all. The old pod serves for
// ten minutes, then goes, and the Service has no ready endpoint. The
// image's CPU stays the cause through every re-title.
func execFormatAfterReleaseRetitled() scenario {
	return scenario{
		expect: expectation{
			Name: "exec-format-error-release-retitle",
			Description: "A release to an amd64-only image crash-loops " +
				"with \"exec format error\" on an arm64 node, some runs " +
				"leave no log line, then the old pod goes and the " +
				"Service has no ready endpoint: the CPU stays the cause.",
			Root: "deployment/shop/reports", Tier: "notify",
			MaxMessages:  3,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: buildArchRetitle,
	}
}

func buildArchRetitle(c *cluster) {
	arm := c.node("n1", "zone-a")
	arm.Labels["kubernetes.io/arch"] = "arm64"
	c.list(arm)
	w := c.deployment("shop", "reports",
		"registry.example.com/reports:3.2", 1)
	old := w.pod(0, "n1")
	slice := clusterSlice(c, "shop", "reports", old)
	c.list(w.objects())
	c.list(old, clusterService(c, "shop", "reports", 8080), slice)
	c.after(5 * time.Minute)
	previous := w.replicaSet
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/reports:3.3"
	})
	c.update(w.deployment, rs)
	served := c.now.Add(10 * time.Minute)
	end := c.now.Add(40 * time.Minute)
	for restarts := int32(1); c.now.Before(end); restarts++ {
		if c.now.After(served) && old != nil {
			// The old pod goes and nothing serves any more.
			zero := int32(0)
			previous.Spec.Replicas = &zero
			previous.Status = appsv1.ReplicaSetStatus{}
			w.setReady(0)
			c.update(w.deployment, previous)
			c.remove(old)
			old = nil
			setEndpointsReady(slice, false)
			c.update(slice)
		}
		archRetitleRun(c, w, restarts)
	}
}

// archRetitleRun is one crash of the new pod; every fifth run leaves
// no error line in its log.
func archRetitleRun(c *cluster, w *workload, restarts int32) {
	pod := w.pod(0, "n1", crashLoop(255, "Error", "", restarts))
	c.update(pod)
	var lines []string
	if restarts%5 != 0 {
		lines = []string{archRetitleLine}
	}
	c.emit(kube.CrashLogObservation(kube.ContainerID(pod.Namespace,
		pod.Name, pod.Spec.Containers[0].Name), c.now, lines))
	c.after(40 * time.Second)
}

// The image's CPU is the cause the evidence proves: the problem title
// and every message lead with it, and the first step checks it, though
// some runs leave no log line and the Service loses its last endpoint
// later. A generic symptom never replaces it.
func TestArchCauseSurvivesRetitles(t *testing.T) {
	result := replayNamed(t, "exec-format-error-release-retitle")
	const cpu = "not built for this node's CPU (arm64)"
	require.NotEmpty(t, result.Messages)
	for _, m := range result.Messages {
		assert.Contains(t, m.Title, cpu)
		require.NotEmpty(t, m.Steps)
		assert.True(t, strings.HasPrefix(m.Steps[0].Text,
			"See which CPU architectures the image is built for"),
			m.Steps[0].Text)
	}
	require.Len(t, result.Incidents, 1)
	assert.Contains(t, result.Incidents[0].Title(), cpu)
}
