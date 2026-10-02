package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreRevision checks a rollout blamed for its pods: if only one
// revision (ReplicaSet) fails while another keeps healthy pods, the
// new revision is the difference. If every revision fails alike, the
// rollout is not what broke.
func scoreRevision(v *view, c *candidate) outcome {
	if c.bestMatch().match.row.Name != "rollout" {
		return outcome{}
	}
	failingRevisions, healthyRevisions := 0, 0
	for _, revision := range v.s.Model.Related(
		c.id, inventory.OwnedBy, inventory.Incoming,
	) {
		if revision.Kind != kube.KindReplicaSet {
			continue
		}
		pods := rootcause.OwnedPods(v.s.Model, revision)
		switch {
		case len(pods) == 0:
		case v.anyFailing(pods):
			failingRevisions++
		default:
			healthyRevisions++
		}
	}
	switch {
	case failingRevisions == 1 && healthyRevisions > 0:
		return outcome{weight: RevisionWeight,
			code: rootcause.ProofNewRevisionFails, text: "only the new " +
				"revision fails; the previous one stays healthy"}
	case failingRevisions > 1 && healthyRevisions == 0:
		return outcome{weight: -RevisionPenalty,
			code: rootcause.ProofEveryRevisionFails,
			text: "every revision fails alike"}
	}
	return outcome{}
}

// anyFailing reports whether one of the pods fails.
func (v *view) anyFailing(pods []inventory.EntityID) bool {
	for _, pod := range sampleIDs(pods) {
		if v.unitFailing(pod) {
			return true
		}
	}
	return false
}
