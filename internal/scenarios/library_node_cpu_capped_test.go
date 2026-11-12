package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cpuCappedScenarios tell a node short of CPU from one container held
// at its own CPU limit, whose waiting tasks the node counts as a stall.
func cpuCappedScenarios() []scenario {
	return []scenario{nodeStallFromCappedContainer(),
		nodeStallFromBusyNode()}
}

// nodeStallFromCappedContainer: a 4-CPU node runs at a tenth of its CPU
// and reports that its tasks stall on CPU half of the time. All of the
// stall is one report container running at its 250m limit. The node has
// CPU to spare; the container's limit is what slows it. Throttling alone
// fails nothing, so it is tracked on the container and sends nothing
// (docs/production-goals.md); a node digest line would be wrong.
func nodeStallFromCappedContainer() scenario {
	return scenario{
		expect: expectation{
			Name: "node-stall-from-capped-container",
			Description: "A node at a tenth of its CPU reports a CPU " +
				"stall that comes from one container held at its " +
				"250m limit.",
			Quiet: true,
		},
		build: func(c *cluster) {
			report, web := cpuCappedFleet(c)
			for range 15 {
				cpuReadings(c, 54, []*corev1.Pod{report, web},
					[]float64{249, 150})
				c.after(time.Minute)
			}
		},
	}
}

// nodeStallFromBusyNode: the twin. The node's pods use nearly all of
// its CPU and its tasks stall on CPU; the report container is at its
// limit too, but the node is what is short.
func nodeStallFromBusyNode() scenario {
	return scenario{
		expect: expectation{
			Name: "node-stall-from-busy-node",
			Description: "A node whose pods use nearly all of its CPU " +
				"reports a CPU stall.",
			Root: "node//n1", Tier: "digest", MaxMessages: 2,
		},
		build: func(c *cluster) {
			report, web := cpuCappedFleet(c)
			for range 15 {
				cpuReadings(c, 54, []*corev1.Pod{report, web},
					[]float64{249, 3600})
				c.after(time.Minute)
			}
		},
	}
}

// cpuCappedFleet is a report pod with a 250m CPU limit and a web pod
// without one, both on the 4-CPU node n1.
func cpuCappedFleet(c *cluster) (report, web *corev1.Pod) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	r := c.deployment("shop", "report",
		"registry.example.com/report:3.1", 1)
	configTemplate(r, func(spec *corev1.PodSpec) {
		spec.Containers[0].Resources.Limits = corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("250m"),
		}
	})
	w := c.deployment("shop", "web", "registry.example.com/web:9", 1)
	c.list(r.objects())
	c.list(w.objects())
	report, web = r.pod(0, "n1"), w.pod(0, "n1")
	c.list(report, web)
	c.after(time.Minute)
	return report, web
}

// cpuReadings is one kubelet poll: the node's CPU stall and the CPU
// each container uses, in millicores.
func cpuReadings(
	c *cluster, stall float64, pods []*corev1.Pod, milli []float64,
) {
	c.emit(inventory.Observation{
		Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
		Entity: inventory.CoreID(kube.KindNode, "", c.n("n1")),
		Attributes: map[string]inventory.Value{
			kube.AttrCPUPSI: inventory.Number(stall),
		},
	})
	for i, pod := range pods {
		c.emit(inventory.Observation{
			Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
			Entity: kube.ContainerID(pod.Namespace, pod.Name, "app"),
			Attributes: map[string]inventory.Value{
				kube.AttrCPUUsageMilli: inventory.Number(milli[i]),
			},
		})
	}
}
