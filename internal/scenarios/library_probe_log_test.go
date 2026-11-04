package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeLogScenarios are running containers whose readiness check the
// application answers with an error status while its log says why.
func probeLogScenarios() []scenario {
	return []scenario{readinessErrorLogged()}
}

// catalogHealthLine is what the application logs each time its own
// readiness check fails.
const catalogHealthLine = "readiness check failed: search index " +
	"not loaded"

// readinessErrorLogged: both replicas of a Deployment run without
// restarting, answer their readiness probe with HTTP 503, and log why on
// every check. The crash-log round reads the current log of each and
// records the last line that names the failure.
func readinessErrorLogged() scenario {
	return scenario{
		expect: expectation{
			Name: "readiness-error-logged",
			Description: "Both replicas of a Deployment run, answer " +
				"their readiness probe with HTTP 503 and log why; the " +
				"Pro message quotes the line.",
			Root: "deployment/shop/catalog", Tier: "notify",
			MaxMessages: 3, Tail: duration(30 * time.Minute),
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: buildReadinessErrorLogged,
	}
}

func buildReadinessErrorLogged(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
	w := c.deployment("shop", "catalog",
		"registry.example.com/catalog:3.1", 2)
	w.setReady(0)
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/ready", Port: intstr.FromInt(8080)}},
		PeriodSeconds: 10, FailureThreshold: 3,
	}
	w.deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = probe
	w.replicaSet.Spec.Template.Spec.Containers[0].ReadinessProbe = probe
	c.list(w.objects())
	started := c.now.Add(-20 * time.Minute)
	nodes := []string{"n1", "n2"}
	for i, node := range nodes {
		c.list(w.pod(i, node, createdAt(started), notReadySince(started)))
	}
	failed := "Readiness probe failed: HTTP probe failed with " +
		"statuscode: 503"
	lines := []string{"info: serving on :8080",
		"warn: " + catalogHealthLine, "info: GET /metrics 200"}
	for count := int32(3); count <= 9; count += 3 {
		c.after(time.Minute)
		for i, node := range nodes {
			pod := w.pod(i, node, createdAt(started),
				notReadySince(started))
			ev := c.warningEvent(pod, "Pod", "Unhealthy", failed,
				"kubelet", count)
			ev.InvolvedObject.FieldPath = "spec.containers{app}"
			c.warn(ev)
			c.emit(kube.ProbeLogObservation(kube.ContainerID(
				pod.Namespace, pod.Name, pod.Spec.Containers[0].Name),
				c.now, lines))
		}
	}
	c.after(5 * time.Minute)
}
