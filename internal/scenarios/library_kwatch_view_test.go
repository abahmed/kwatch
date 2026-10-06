package scenarios

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kwatchViewScenarios are findings about what kwatch itself can see.
func kwatchViewScenarios() []scenario {
	return []scenario{kwatchNetworkRestricted(), kubeletReachedRarely()}
}

// kwatchNetworkRestricted: every probed dependency fails in the same
// rounds, which says more about kwatch's network than about five
// dependencies. One digest finding, and no dependency is blamed.
func kwatchNetworkRestricted() scenario {
	return scenario{
		expect: expectation{
			Name: "kwatch-network-restricted",
			Description: "All five probed dependencies fail in every " +
				"probe round.",
			Root: "kwatch//kwatch", Tier: "digest", MaxMessages: 2,
			Tail: duration(45 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			for range 6 {
				c.emit(inventory.Observation{
					Kind: inventory.Observed, Source: "active-probe-dependency",
					At: c.now, Entity: kube.KwatchSelf,
					Attributes: map[string]inventory.Value{
						kube.AttrDependenciesUnreachable: inventory.Number(5),
						kube.AttrFailureDuration:         inventory.Number(90),
					},
				})
				c.after(30 * time.Second)
			}
		},
	}
}

// kubeletReachedRarely: kwatch failed to read a Ready node's kubelet
// four times in six hours.
func kubeletReachedRarely() scenario {
	return scenario{
		expect: expectation{
			Name: "kubelet-unreachable-window",
			Description: "A Ready node's kubelet stats could not be read " +
				"four times in six hours.",
			Root: "node//n1", Tier: "digest", MaxMessages: 2,
			Tail: duration(45 * time.Minute),
		},
		build: func(c *cluster) {
			node := c.node("n1", "zone-a")
			c.list(node)
			c.emit(inventory.Observation{
				Kind: inventory.Observed, Source: kube.StatsSource,
				At: c.now, Entity: inventory.CoreID(kube.KindNode, "", c.n("n1")),
				Attributes: map[string]inventory.Value{
					kube.AttrKubeletFailures:    inventory.Number(4),
					kube.AttrKubeletFailureSpan: inventory.Number(7200),
				},
			})
			c.after(10 * time.Minute)
		},
	}
}
