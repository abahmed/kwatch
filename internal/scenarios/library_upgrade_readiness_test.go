package scenarios

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// upgradeReadinessScenarios are digests that also answer "is it safe to
// upgrade?".
func upgradeReadinessScenarios() []scenario {
	return []scenario{upgradeReadinessDigest()}
}

// upgradeReadinessDigest: a deprecated API is still requested (a digest
// incident) and a PodDisruptionBudget currently allows no disruption. The
// digest lists the API as an incident and adds one readiness line for the
// budget; it does not say the API twice.
func upgradeReadinessDigest() scenario {
	api := kube.DeprecatedAPI{Group: "policy", Version: "v1beta1",
		Resource: "poddisruptionbudgets", Removed: "1.25"}
	pdb := inventory.EntityID{Kind: kube.KindPDB, Namespace: "shop",
		Name: "api-pdb"}
	return scenario{
		expect: expectation{
			Name: "upgrade-readiness-digest",
			Description: "A deprecated API is still requested and a " +
				"PodDisruptionBudget allows no disruption; the digest " +
				"adds one upgrade-readiness line for the budget.",
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
			}, inventory.Observation{
				Kind: inventory.Observed, Source: "kubernetes",
				At: c.now, Entity: pdb,
				Attributes: map[string]inventory.Value{
					kube.AttrDisruptionsAllowed: inventory.Number(0),
					kube.AttrExpectedPods:       inventory.Number(2),
					kube.AttrCurrentHealthy:     inventory.Number(2),
					kube.AttrDesiredHealthy:     inventory.Number(2),
				},
			})
			attrs := map[string]inventory.Value{
				kube.AttrDeprecatedGroup:    inventory.Text(api.Group),
				kube.AttrDeprecatedVersion:  inventory.Text(api.Version),
				kube.AttrDeprecatedResource: inventory.Text(api.Resource),
				kube.AttrDeprecatedRemoved:  inventory.Text(api.Removed),
			}
			for range 6 {
				c.emit(inventory.Observation{
					Kind: inventory.Observed, Source: kube.ProbeSource,
					At: c.now, Entity: api.ID(), Attributes: attrs,
				})
				c.after(30 * time.Second)
			}
		},
	}
}
