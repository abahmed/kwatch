package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// memoryContextScenarios are OOM kills told with the history kwatch
// keeps, a kill with no history at all, and probes that time out with
// and without the container at its CPU limit.
func memoryContextScenarios() []scenario {
	return []scenario{oomFlatAtLimit(), oomWithoutHistory(),
		readinessTimeoutAtCPULimit(), livenessTimeoutUnderCPULimit(),
		oomAtLimitRangeCap()}
}

// oomFlatAtLimit: an API's memory sat near its 512Mi limit for hours
// and each of its last runs was killed there. kwatch has five hours of
// history; the message gives the peak against limit and request over
// those five hours, and says the real need is not known.
func oomFlatAtLimit() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-flat-at-limit-sizing",
			Description: "A container stayed near its 512Mi limit for " +
				"hours and was OOM-killed there run after run; five " +
				"hours of usage history exist.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			pod := buildOOMAtLimit(c)
			c.emit(oomHistory(c, pod, map[string]inventory.Value{
				kube.AttrMemoryPeak24h:      inventory.Number(510 * mib),
				kube.AttrMemoryPrevStart:    inventory.Number(480 * mib),
				kube.AttrMemoryPrevPeak:     inventory.Number(510 * mib),
				kube.AttrMemoryPrevDrawdown: inventory.Number(3),
				kube.AttrMemoryPrevSeconds:  inventory.Number(4 * 3600),
				kube.AttrMemoryPrevEnded: inventory.Time(
					c.now.Add(-30 * time.Second)),
				kube.AttrMemoryHistorySince: inventory.Time(
					c.now.Add(-5 * time.Hour).Truncate(time.Hour)),
				kube.AttrMemoryPeakHours: inventory.Number(4),
				kube.AttrMemoryRunsEnded: inventory.Number(3),
				kube.AttrMemoryRunsLow:   inventory.Number(500 * mib),
				kube.AttrMemoryRunsHigh:  inventory.Number(510 * mib),
			}))
			c.after(time.Minute)
		},
	}
}

// oomWithoutHistory: the same kill with no kubelet reading at all, as
// when the container died before the first sample. Nothing about its
// use is said.
func oomWithoutHistory() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-without-usage-history",
			Description: "A container is OOM-killed before kwatch read " +
				"its memory even once; no usage is stated.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			buildOOMAtLimit(c)
			c.after(time.Minute)
		},
	}
}

// oomAtLimitRangeCap: the same kill in a namespace whose LimitRange
// caps a container at 512Mi, the limit it has: the fix says the cap
// blocks the raise instead of offering a value the cluster refuses.
func oomAtLimitRangeCap() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-at-limitrange-cap",
			Description: "A container is OOM-killed at its 512Mi limit " +
				"in a namespace whose LimitRange allows no more.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(&corev1.LimitRange{
				ObjectMeta: clusterMeta(c, "shop", "limits", "limitrange"),
				Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
					Type: corev1.LimitTypeContainer,
					Max: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("512Mi")},
				}}},
			})
			pod := buildOOMAtLimit(c)
			c.emit(oomHistory(c, pod, map[string]inventory.Value{
				kube.AttrMemoryPeak24h: inventory.Number(508 * mib),
			}))
			c.after(time.Minute)
		},
	}
}

const mib = float64(1 << 20)

// buildOOMAtLimit lists shop/api crash looping on OOM kills, with a
// 512Mi limit and a 128Mi request, and returns its pod.
func buildOOMAtLimit(c *cluster) *corev1.Pod {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:2.0", 1)
	w.setReady(0)
	c.list(w.objects())
	c.after(time.Minute)
	pod := w.pod(0, "n1", crashLoop(137, "OOMKilled", "", 4),
		longRun(4*time.Hour), memoryRequest("128Mi"))
	c.list(pod)
	return pod
}

// memoryRequest sets the container's memory request.
func memoryRequest(q string) podState {
	return func(_ *cluster, pod *corev1.Pod) {
		pod.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(q),
		}
	}
}

// oomHistory is the memory history the stats poller publishes.
func oomHistory(
	c *cluster, pod *corev1.Pod, attrs map[string]inventory.Value,
) inventory.Observation {
	return inventory.Observation{
		Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
		Entity:     kube.ContainerID(pod.Namespace, pod.Name, "app"),
		Attributes: attrs,
	}
}

// readinessTimeoutAtCPULimit: the liveness probe times out while each
// web container uses 198m of its 200m CPU limit; the kernel throttles
// it only lightly, so the throttled share alone says little.
func readinessTimeoutAtCPULimit() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-timeout-at-cpu-limit",
			Description: "Liveness probes time out while the containers " +
				"run at their 200m CPU limit with little throttling.",
			Root: "deployment/shop/web", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) { buildCPURounds(c, 198, 20) },
	}
}

// livenessTimeoutUnderCPULimit: the same timeouts with the containers
// far under their limit; no CPU line is added.
func livenessTimeoutUnderCPULimit() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-timeout-under-cpu-limit",
			Description: "Liveness probes time out while the containers " +
				"use a quarter of their CPU limit; CPU is not mentioned.",
			Root: "deployment/shop/web", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) { buildCPURounds(c, 50, 2) },
	}
}

// buildCPURounds runs liveness kill rounds of shop/web with the given
// CPU use (millicores) and throttled share read each round.
func buildCPURounds(c *cluster, milli, throttled float64) {
	w := throttledWorkload(c)
	c.after(time.Minute)
	for round := int32(3); round <= 10; round++ {
		c.after(40 * time.Second)
		livenessKillRound(c, w, round)
		throttleReading(c, w, throttled)
		for i, node := range []string{"n1", "n2"} {
			pod := w.pod(i, node)
			c.emit(oomHistory(c, pod, map[string]inventory.Value{
				kube.AttrCPUUsageMilli: inventory.Number(milli),
			}))
		}
	}
}
