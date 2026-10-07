package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeThrottleScenarios are failing probes whose message can say the
// container was starved of CPU.
func probeThrottleScenarios() []scenario {
	return []scenario{livenessTimeoutThrottled()}
}

// livenessTimeoutThrottled: a 200m CPU limit throttles every web pod
// 72% of the time, so the health endpoint misses its probe deadline and
// the kubelet kills the pods in a loop. The message says the probe timed
// out while the container was throttled. The same loop without the
// throttling reading is liveness-kill-loop.
func livenessTimeoutThrottled() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-timeout-cpu-throttled",
			Description: "Liveness probes time out and the kubelet " +
				"kills the pods in a loop while their 200m CPU limit " +
				"throttles them 72% of the time.",
			Root: "deployment/shop/web", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := throttledWorkload(c)
			c.after(time.Minute)
			for round := int32(3); round <= 10; round++ {
				c.after(40 * time.Second)
				livenessKillRound(c, w, round)
				throttleReading(c, w, 72)
			}
		},
	}
}

// throttledWorkload is shop/web with a liveness probe and a 200m CPU
// limit on its only container.
func throttledWorkload(c *cluster) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "web", "registry.example.com/web:9", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/healthz",
					Port: intstr.FromInt32(8080)}},
			PeriodSeconds: 10, FailureThreshold: 3,
		}
		spec.Containers[0].Resources.Limits = corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("200m"),
		}
	})
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	return w
}

// throttleReading is the stats poller's cAdvisor reading of how long
// each pod's container was CPU-throttled.
func throttleReading(c *cluster, w *workload, pct float64) {
	for i, node := range []string{"n1", "n2"} {
		pod := w.pod(i, node)
		c.emit(inventory.Observation{
			Kind: inventory.Observed, Source: "cadvisor", At: c.now,
			Entity: kube.ContainerID(pod.Namespace, pod.Name, "app"),
			Attributes: map[string]inventory.Value{
				kube.AttrThrottledPct: inventory.Number(pct),
			},
		})
	}
}
