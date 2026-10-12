package explain

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A rollout and an edit of a config its pods read when a container
// starts, made between two revisions, split the pods the same way: the
// new revision's pods are exactly those that read the new content. A
// split by revision and a split by config version are then one fact, so
// neither is evidence for its own candidate over the other.

// revisionBand is the span in which an edit reaches only the newest
// revision's pods: after the previous ReplicaSet was created until the
// newest one was. Revisions are ordered by creation time.
func (v *view) revisionBand(
	deployment inventory.EntityID,
) (from, until time.Time, ok bool) {
	var created []time.Time
	for _, id := range v.s.Model.Related(
		deployment, inventory.OwnedBy, inventory.Incoming,
	) {
		if id.Kind != kube.KindReplicaSet {
			continue
		}
		if entity, found := v.s.Model.Entity(id); found {
			if at := attributeTime(entity, kube.AttrCreated); !at.IsZero() {
				created = append(created, at)
			}
		}
	}
	if len(created) < 2 {
		return from, until, false
	}
	slices.SortFunc(created, func(a, b time.Time) int { return b.Compare(a) })
	return created[1], created[0].Add(TemporalSlack), true
}

// splitsWithConfig reports a workload whose pods read, at start, a
// ConfigMap or Secret edited inside its newest revision's band. With
// config set only that object counts.
func (v *view) splitsWithConfig(
	workload, config inventory.EntityID,
) bool {
	from, until, ok := v.revisionBand(workload)
	if !ok {
		return false
	}
	for _, pod := range sampleIDs(v.ownedPods(workload)) {
		entity, found := v.s.Model.Entity(pod)
		if !found {
			continue
		}
		for _, ref := range kube.FrozenConfigs(entity) {
			if config != (inventory.EntityID{}) && ref != config {
				continue
			}
			at := v.latestChange(ref)
			if !at.IsZero() && !at.Before(from) && !at.After(until) {
				return true
			}
		}
	}
	return false
}
