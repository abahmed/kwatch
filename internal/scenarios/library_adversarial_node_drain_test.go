package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// advOOMTwoVictims: on n1, under memory pressure, ledger is killed at
// 500Mi of its 512Mi limit (its limit is too low) and cart is killed at
// 140Mi (the node ran out because an importer with a huge limit holds
// the memory). Two verdicts: ledger's limit; the node for cart.
func advOOMTwoVictims() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-oom-two-victims",
			Description: "Two containers are OOM-killed on one node under " +
				"memory pressure: one at its own limit, one far below.",
			Root: "deployment/billing/ledger", Tier: "notify",
			OtherRoots:  []string{"node//n1"},
			MaxMessages: 5, Tail: duration(15 * time.Minute),
			MustNotBlame: []string{"deployment/shop/cart"},
		},
		build: buildAdvTwoVictims,
	}
}

func buildAdvTwoVictims(c *cluster) {
	n1 := c.node("n1", "zone-a")
	setNodeCondition(n1, corev1.NodeMemoryPressure, corev1.ConditionTrue,
		"KubeletHasInsufficientMemory", "memory is short",
		c.start.Add(-time.Minute))
	c.list(n1, c.node("n2", "zone-a"))
	ledger := c.deployment("billing", "ledger",
		"registry.example.com/ledger:2.0", 1)
	cart := c.deployment("shop", "cart", "registry.example.com/cart:5", 1)
	importer := c.deployment("batch", "importer",
		"registry.example.com/importer:1.0", 1)
	ledger.setReady(0)
	cart.setReady(0)
	for _, w := range []*workload{ledger, cart, importer} {
		c.list(w.objects())
	}
	c.after(time.Minute)
	lp := ledger.pod(0, "n1", crashLoop(137, "OOMKilled", "", 4),
		longRun(2*time.Hour))
	cp := cart.pod(0, "n1", crashLoop(137, "OOMKilled", "", 4),
		longRun(2*time.Hour))
	hog := importer.pod(0, "n1")
	hog.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("24Gi"),
	}
	c.list(lp, cp, hog)
	mib := float64(1 << 20)
	peak := func(p *corev1.Pod, usedMi float64) inventory.Observation {
		return inventory.Observation{
			Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
			Entity: kube.ContainerID(p.Namespace, p.Name, "app"),
			Attributes: map[string]inventory.Value{
				kube.AttrMemoryPeak24h: inventory.Number(usedMi * mib),
			},
		}
	}
	c.emit(usage(c.now, kube.ContainerID(hog.Namespace, hog.Name, "app"),
		3174*mib), peak(lp, 500), peak(cp, 140))
	event := c.warningEvent(n1, "Node", "SystemOOM",
		"System OOM encountered, victim process: importer, pid: 4242",
		"kernel-monitor", 1)
	event.InvolvedObject.UID = types.UID(n1.Name)
	c.warn(event)
	c.after(10 * time.Minute)
}

// advCleanDrainQuiet: a node is cordoned and drained for an upgrade; each
// pod is evicted and replaced within twenty seconds on another node, and
// the node is removed. A planned drain that works is not news.
func advCleanDrainQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-clean-drain-quiet",
			Description: "A node is cordoned, drained pod by pod with " +
				"replacements ready in seconds, and removed.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			n2 := c.node("n2", "zone-a")
			c.list(c.node("n1", "zone-a"), n2, c.node("n3", "zone-a"))
			fleet := nodeFleet(c, "shop", []string{"cart", "search"},
				[]string{"n1", "n2", "n3"})
			c.after(5 * time.Minute)
			cordoned := last(c, n2)
			cordoned.Spec.Unschedulable = true
			cordoned.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
			}}
			c.update(cordoned)
			for i, w := range fleet {
				c.after(30 * time.Second)
				victim := w.pod(1, "n2")
				c.remove(victim)
				w.setReady(2)
				c.update(w.objects())
				c.create(w.pod(3+i, "n1", startedNow, notReady))
				c.after(15 * time.Second)
				c.update(w.pod(3+i, "n1", startedNow))
				w.setReady(3)
				c.update(w.objects())
			}
			c.after(2 * time.Minute)
			c.remove(last(c, n2))
		},
	}
}

// advPDBZeroQuiet: a budget on a single-replica Deployment allows no
// disruption, as it always does; a different node is cordoned and has
// nothing the budget protects. Nothing is stuck.
func advPDBZeroQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-pdb-zero-quiet",
			Description: "A budget that allows no disruption sits beside " +
				"a cordoned node that holds none of its pods.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			n3 := c.node("n3", "zone-a")
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), n3)
			ledger := c.deployment("shop", "ledger",
				"registry.example.com/ledger:9", 1)
			c.list(ledger.objects())
			c.list(ledger.pod(0, "n1"))
			c.list(lifecycleBudget(c, ledger, 0))
			c.after(time.Minute)
			cordoned := last(c, n3)
			cordoned.Spec.Unschedulable = true
			cordoned.Spec.Taints = []corev1.Taint{{
				Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
			}}
			c.update(cordoned)
			for range 15 {
				c.after(time.Minute)
				c.update(lifecycleBudget(c, ledger, 0))
			}
		},
	}
}
