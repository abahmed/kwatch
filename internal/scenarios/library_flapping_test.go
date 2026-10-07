package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// flappingScenarios are workloads whose pods switch Ready on and off.
func flappingScenarios() []scenario {
	return []scenario{readinessFlapping(), startupReadinessChurn()}
}

// flapProbe is what the kubelet reports when a pod's readiness probe
// times out.
const flapProbe = "Readiness probe failed: Get " +
	"\"http://10.0.1.5:8080/ready\": context deadline exceeded"

// readinessFlapping: both replicas of the shop API pass and fail their
// readiness probe over and over for five minutes, and never restart.
// The Service in front of them keeps changing its endpoints. The API
// is the root.
func readinessFlapping() scenario {
	return scenario{
		expect: expectation{
			Name: "readiness-flapping",
			Description: "Two API pods switch between ready and not " +
				"ready nine times in five minutes without restarting.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1", "service/shop/api"},
			Tail:         duration(5 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api",
				"registry.example.com/api:5", 2)
			probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/ready",
					Port: intstr.FromInt(8080)}}, PeriodSeconds: 5}
			w.deployment.Spec.Template.Spec.Containers[0].ReadinessProbe =
				probe
			w.replicaSet.Spec.Template.Spec.Containers[0].ReadinessProbe =
				probe
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"),
				clusterService(c, "shop", "api", 8080),
				trafficSlice(c, "api", w.pod(0, "n1"), w.pod(1, "n1")))
			c.after(20 * time.Minute)
			unready := map[int]bool{}
			for i := range 10 {
				replica := i % 2
				pod := w.pod(replica, "n1")
				unready[replica] = (i/2)%2 == 0
				if unready[replica] {
					pod = w.pod(replica, "n1", notReady)
				}
				c.update(pod)
				ready := int32(2)
				for _, down := range unready {
					if down {
						ready--
					}
				}
				w.setReady(ready)
				c.update(w.objects())
				if unready[replica] {
					c.warn(c.warningEvent(pod, "Pod", "Unhealthy",
						flapProbe, "kubelet", int32(i+1)))
				}
				c.after(30 * time.Second)
			}
		},
	}
}

// startupReadinessChurn: a rollout replaces eight replicas, and each
// new pod is not ready for a while before it becomes ready. That is
// starting up, not flapping.
func startupReadinessChurn() scenario {
	return scenario{
		expect: expectation{
			Name: "startup-readiness-churn",
			Description: "Eight new pods of a rollout each become ready " +
				"for the first time within a minute.",
			Quiet: true,
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api",
				"registry.example.com/api:5", 8)
			c.list(w.objects())
			c.after(time.Minute)
			for i := range 8 {
				c.create(w.pod(i, "n1", startedNow, notReady))
			}
			c.after(40 * time.Second)
			for i := range 8 {
				c.update(w.pod(i, "n1", startedNow))
			}
			c.after(10 * time.Minute)
		},
	}
}
