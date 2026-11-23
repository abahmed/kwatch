package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
)

const (
	// scaleFlapWindow is how far back a replica change still counts.
	scaleFlapWindow = 30 * time.Minute
	// scaleFlapMinChanges is how many changes of the replica count
	// inside scaleFlapWindow make a flapping count.
	scaleFlapMinChanges = 6
)

// targetEntity is the model's entity for id, or an empty one.
func targetEntity(
	ctx detection.Context, id inventory.EntityID,
) inventory.Entity {
	if ctx.Model == nil {
		return inventory.Entity{}
	}
	e, _ := ctx.Model.Entity(id)
	return e
}

// replicasFlapping reports a workload whose spec.replicas changed at
// least scaleFlapMinChanges times in scaleFlapWindow. It is read for
// the workload and for the autoscaler that scales it, so the incident
// can name either. A scale to or from zero is a decision, not a flap.
func replicasFlapping(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if ctx.Model == nil || e.ID.Name == "" {
		return detection.Finding{}, false
	}
	var first time.Time
	n := 0
	for _, change := range ctx.Model.Changes(e.ID,
		ctx.Now.Add(-scaleFlapWindow)) {
		for _, f := range change.Fields {
			if f.Path != "spec.replicas" || f.Before == "0" ||
				f.After == "0" {
				continue
			}
			if n == 0 {
				first = change.At
			}
			n++
		}
	}
	if n < scaleFlapMinChanges {
		return detection.Finding{}, false
	}
	ctx.RecheckAfter(first.Add(scaleFlapWindow).Sub(ctx.Now) +
		time.Nanosecond)
	return detection.Finding{
		Reason: reasons.ReplicasFlapping, Severity: detection.Info,
		Since: first,
		Summary: "Replicas of " + e.ID.Name + " changed " +
			strconv.Itoa(n) + " times in " +
			format.Duration(scaleFlapWindow),
	}, true
}

// scaleTargets are the workloads the autoscaler id scales.
func scaleTargets(
	ctx detection.Context, id inventory.EntityID,
) []inventory.EntityID {
	if ctx.Model == nil {
		return nil
	}
	return ctx.Model.Related(id, inventory.Scales, inventory.Outgoing)
}
