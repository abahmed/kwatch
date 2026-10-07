package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeOOMScenarios tell a kill by the node running out of memory from a
// kill by the container's own limit.
func nodeOOMScenarios() []scenario {
	return []scenario{oomByNode(), oomAtOwnLimitDuringNodeOOM(),
		oomByOvercommittedNode(), limitsOvercommittedQuiet()}
}

// oomByNode: the node's kernel kills api at 180Mi of its 512Mi limit
// because a batch importer without a limit ate the node's memory. The
// message names the node and the importer instead of advising a higher
// limit for api.
func oomByNode() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-by-node",
			Description: "A container is OOM-killed far below its own " +
				"limit while the node's kernel reports a system OOM " +
				"and an importer without a limit holds the memory.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildNodeOOM(c, 180) },
	}
}

// oomAtOwnLimitDuringNodeOOM: the same system OOM, but api used 505Mi
// of 512Mi. It hit its own limit, so the node is not blamed.
func oomAtOwnLimitDuringNodeOOM() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-at-own-limit-during-node-oom",
			Description: "A container is OOM-killed at its own limit " +
				"while the node also reports a system OOM; the kill " +
				"stays the container's.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) { buildNodeOOM(c, 505) },
	}
}

// oomByOvercommittedNode: the same kill, on a node under memory pressure
// whose pods' limits add up to 187% of its memory. The overcommit is
// named as why a pod inside its own limit died.
func oomByOvercommittedNode() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-by-overcommitted-node",
			Description: "A container is OOM-killed far below its own " +
				"limit on a node under memory pressure whose pods' " +
				"limits add up to far more than its memory.",
			Root: "node//n1", Tier: "notify", MaxMessages: 3,
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildNodeOOMWith(c, 180, "30Gi", true) },
	}
}

// limitsOvercommittedQuiet: limits add up to 187% of the node's memory
// and 3x its CPU, and every pod runs fine. Burstable limits are normal;
// nothing is broken, so nothing is said.
func limitsOvercommittedQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "limits-overcommitted-quiet",
			Description: "Pod limits far above the node's capacity while " +
				"nothing fails stay unreported.",
			Quiet: true, MaxMessages: 0,
			Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			web := c.deployment("shop", "web",
				"registry.example.com/web:1.0", 1)
			c.list(web.objects())
			c.after(time.Minute)
			pod := web.pod(0, "n1")
			pod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("30Gi"),
				corev1.ResourceCPU:    resource.MustParse("12"),
			}
			c.list(pod)
			c.after(time.Minute)
		},
	}
}

func buildNodeOOM(c *cluster, usedMi float64) {
	buildNodeOOMWith(c, usedMi, "", false)
}

// buildNodeOOMWith is buildNodeOOM with the importer's memory limit set
// to hogLimit (when not empty) and the node reporting MemoryPressure.
func buildNodeOOMWith(
	c *cluster, usedMi float64, hogLimit string, pressure bool,
) {
	node := c.node("n1", "zone-a")
	if pressure {
		setNodeCondition(node, corev1.NodeMemoryPressure,
			corev1.ConditionTrue, "KubeletHasInsufficientMemory",
			"memory is short", c.start.Add(-time.Minute))
	}
	c.list(node)
	api := c.deployment("shop", "api", "registry.example.com/api:2.0", 1)
	api.setReady(0)
	importer := c.deployment("batch", "importer",
		"registry.example.com/importer:1.0", 1)
	c.list(api.objects())
	c.list(importer.objects())
	c.after(time.Minute)
	victim := api.pod(0, "n1", crashLoop(137, "OOMKilled", "", 4),
		longRun(2*time.Hour))
	hog := importer.pod(0, "n1")
	if hogLimit != "" {
		hog.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(hogLimit),
		}
	}
	c.list(victim, hog)
	mib := float64(1 << 20)
	c.emit(
		usage(c.now, kube.ContainerID(hog.Namespace, hog.Name, "app"),
			3174*mib),
		inventory.Observation{
			Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
			Entity: kube.ContainerID(victim.Namespace, victim.Name, "app"),
			Attributes: map[string]inventory.Value{
				kube.AttrMemoryPeak24h: inventory.Number(usedMi * mib),
			},
		})
	event := c.warningEvent(node, "Node", "SystemOOM",
		"System OOM encountered, victim process: importer, pid: 4242",
		"kernel-monitor", 1)
	event.InvolvedObject.UID = types.UID(node.Name)
	c.warn(event)
	c.after(time.Minute)
}

// usage is a kubelet reading of one container's working set.
func usage(
	at time.Time, id inventory.EntityID, workingSet float64,
) inventory.Observation {
	return inventory.Observation{
		Kind: inventory.Observed, Source: kube.StatsSource, At: at,
		Entity: id, Attributes: map[string]inventory.Value{
			kube.AttrMemoryWorking: inventory.Number(workingSet),
		},
	}
}
