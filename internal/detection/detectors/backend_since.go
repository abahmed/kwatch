package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// backendDownSince is when a webhook's backend Service stopped serving,
// by the cluster's record: the deletion of the Service kwatch saw, or,
// when the workload behind it is at zero replicas, when it was scaled
// there (see scaledSince). Zero when the cluster does not say, so a
// backend that was gone before kwatch looked is not dated by kwatch's
// own first sight of it.
func backendDownSince(
	ctx detection.Context, service inventory.EntityID,
) time.Time {
	if !ctx.Model.Exists(service) {
		return deletedAt(ctx, service)
	}
	controller := backendController(ctx, service)
	e, ok := ctx.Model.Entity(controller)
	if controller == service || !ok {
		return time.Time{}
	}
	if replicas, known := number(e, kube.AttrReplicas); !known ||
		replicas != 0 {
		return time.Time{}
	}
	if since, known := scaledSince(e, scaledBy(ctx, controller)); known {
		return since
	}
	return time.Time{}
}

// deletedAt is when the latest deletion of id that the model recorded
// happened, or zero.
func deletedAt(ctx detection.Context, id inventory.EntityID) time.Time {
	changes := ctx.Model.Changes(id, time.Time{})
	for i := len(changes) - 1; i >= 0; i-- {
		if changes[i].Deleted {
			return changes[i].At
		}
	}
	return time.Time{}
}
