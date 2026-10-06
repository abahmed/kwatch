package scenarios

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// deprecatedAPIScenarios are API versions that something still calls.
func deprecatedAPIScenarios() []scenario {
	return []scenario{deprecatedAPIStillCalled()}
}

// deprecatedAPIStillCalled: the API server reports that
// policy/v1beta1 poddisruptionbudgets was requested, and the next minor
// upgrade removes it. Nothing is failing now, so it is one digest line.
func deprecatedAPIStillCalled() scenario {
	api := kube.DeprecatedAPI{Group: "policy", Version: "v1beta1",
		Resource: "poddisruptionbudgets", Removed: "1.25"}
	return scenario{
		expect: expectation{
			Name: "deprecated-api-still-called",
			Description: "The API server reports a request for " +
				"policy/v1beta1 poddisruptionbudgets, removed in the " +
				"next minor upgrade.",
			Root: "deprecated-api//policy/v1beta1 poddisruptionbudgets",
			Tier: "digest", MaxMessages: 2, Tail: duration(45 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.emit(inventory.Observation{
				Kind: inventory.Observed, Source: kube.ProbeSource,
				At: c.now, Entity: kube.APIServer,
				Attributes: map[string]inventory.Value{
					kube.AttrHealthy:     inventory.Bool(true),
					kube.AttrLatencyMS:   inventory.Number(3),
					kube.AttrServerMinor: inventory.Number(24),
				},
			})
			for range 6 {
				c.emit(inventory.Observation{
					Kind: inventory.Observed, Source: kube.ProbeSource,
					At: c.now, Entity: api.ID(),
					Attributes: map[string]inventory.Value{
						kube.AttrDeprecatedGroup:    inventory.Text(api.Group),
						kube.AttrDeprecatedVersion:  inventory.Text(api.Version),
						kube.AttrDeprecatedResource: inventory.Text(api.Resource),
						kube.AttrDeprecatedRemoved:  inventory.Text(api.Removed),
					},
				})
				c.after(30 * time.Second)
			}
		},
	}
}
