package detectors

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultRolloutStuck is how long a StatefulSet rollout may make no
// progress. It replaces one pod at a time and waits for each to be ready.
const DefaultRolloutStuck = 10 * time.Minute

// statefulSetRollout reports a StatefulSet whose update revision has not
// replaced its current revision: a partition holding ordinals back, or
// updated pods that stopped progressing (usually one ordinal that never
// becomes ready). OnDelete StatefulSets only roll when pods are deleted.
func statefulSetRollout(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	update := text(e, kube.AttrRevision)
	current := text(e, kube.AttrCurrentRevision)
	if update == "" || current == "" || update == current ||
		text(e, kube.AttrUpdateStrategy) == "OnDelete" {
		return detection.Finding{}, false
	}
	desired, _ := number(e, kube.AttrReplicas)
	updated, _ := number(e, kube.AttrUpdatedReplicas)
	partition, _ := number(e, kube.AttrPartition)
	evidence := rolloutEvidence(e, updated, partition)
	if partition > 0 && updated >= desired-partition {
		return detection.Finding{
			Reason: reasons.StatefulSetRolloutStuck, Severity: detection.Info,
			Since: valueSince(e, kube.AttrPartition),
			Summary: "Rollout is held by partition " + countText(partition) +
				": ordinals below it keep revision " + current,
			Evidence: evidence,
		}, true
	}
	since := latest(valueSince(e, kube.AttrRevision),
		valueSince(e, kube.AttrUpdatedReplicas))
	if !sustained(ctx, "rollout-stuck", since, DefaultRolloutStuck) {
		return detection.Finding{}, false
	}
	summary := "Rollout to revision " + update + " is stuck: " +
		countText(updated) + " of " + countText(desired) + " pods updated"
	if blocked := unreadyPods(ctx, e.ID); len(blocked) > 0 {
		summary += "; " + strings.Join(blocked, ", ") + " not ready"
		for _, name := range blocked {
			evidence = append(evidence,
				detection.Evidence{Label: "not ready", Value: name})
		}
	}
	return detection.Finding{
		Reason: reasons.StatefulSetRolloutStuck, Severity: detection.Warning,
		Health: detection.Failing, Since: since, Summary: summary,
		Evidence: evidence,
	}, true
}

func rolloutEvidence(
	e inventory.Entity, updated, partition float64,
) []detection.Evidence {
	current, _ := number(e, kube.AttrCurrentReplicas)
	return []detection.Evidence{
		{Label: "current revision", Value: text(e, kube.AttrCurrentRevision)},
		{Label: "update revision", Value: text(e, kube.AttrRevision)},
		{Label: "current replicas", Value: countText(current)},
		{Label: "updated replicas", Value: countText(updated)},
		{Label: "partition", Value: countText(partition)},
	}
}

// unreadyPods names the owned pods that are not ready, in name order.
func unreadyPods(
	ctx detection.Context, owner inventory.EntityID,
) []string {
	if ctx.Model == nil {
		return nil
	}
	var out []string
	for _, id := range ctx.Model.Related(
		owner, inventory.OwnedBy, inventory.Incoming,
	) {
		pod, ok := ctx.Model.Entity(id)
		if ok && id.Kind == kube.KindPod && !flag(pod, kube.AttrReady) {
			out = append(out, id.Name)
		}
	}
	sort.Strings(out)
	return out
}

func countText(value float64) string {
	return strconv.Itoa(int(value))
}
