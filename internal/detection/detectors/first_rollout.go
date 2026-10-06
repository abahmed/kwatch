package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// firstRolloutSlack is how long after the Deployment's creation its
// Available condition may have been set and still be the one the
// controller wrote at creation. Later, the Deployment had been
// available and stopped being so.
const firstRolloutSlack = 2 * time.Minute

// neverHealthyEvidence marks a Deployment whose first rollout has not
// become healthy yet: it has had one ReplicaSet, ever, and has not been
// available since it was created. That is a different story from a
// workload that worked and broke, and the message says so.
func neverHealthyEvidence(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	created := timestamp(e, kube.AttrCreated)
	if e.ID.Kind != kube.KindDeployment || ctx.Model == nil ||
		created.IsZero() || !ctx.Synced(kube.KindReplicaSet) ||
		replicaSetsOf(ctx, e.ID) != 1 {
		return nil
	}
	status, _, since := condition(e, "Available")
	if status == "True" || since.IsZero() ||
		since.Sub(created) > firstRolloutSlack {
		return nil
	}
	return []detection.Evidence{{Label: detection.EvidenceNeverHealthy,
		Value: created.UTC().Format(time.RFC3339)}}
}

func replicaSetsOf(ctx detection.Context, deployment inventory.EntityID) int {
	count := 0
	for _, id := range ctx.Model.Related(deployment, inventory.OwnedBy,
		inventory.Incoming) {
		if id.Kind == kube.KindReplicaSet {
			count++
		}
	}
	return count
}
