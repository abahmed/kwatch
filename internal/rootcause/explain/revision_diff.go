package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// newestRevisions returns a Deployment's two newest ReplicaSets, newest
// first, with their revision numbers. ok is false for a Deployment with
// fewer than two known revisions.
func newestRevisions(
	model inventory.Reader, deployment inventory.EntityID,
) (newest, previous inventory.Entity, revision int, ok bool) {
	var best, second int
	for _, id := range model.Related(
		deployment, inventory.OwnedBy, inventory.Incoming,
	) {
		n, known := revisionOf(model, id)
		entity, present := model.Entity(id)
		if !known || !present {
			continue
		}
		switch {
		case n > best:
			previous, newest = newest, entity
			best, second = n, best
		case n > second:
			previous, second = entity, n
		}
	}
	return newest, previous, best, second > 0
}

// revisionEdits names what the Deployment's newest revision changed in
// its pod template compared with the one before: the template edits
// recorded between the creation of the previous ReplicaSet and the
// newest one, ranked likeliest culprit first. It is empty when kwatch
// did not see the edit (it started after the rollout).
func (v *view) revisionEdits(
	deployment inventory.EntityID,
) (int, []inventory.FieldChange) {
	newest, previous, revision, ok := newestRevisions(v.s.Model, deployment)
	if !ok {
		return 0, nil
	}
	from := attributeTime(previous, kube.AttrCreated)
	until := attributeTime(newest, kube.AttrCreated).Add(TemporalSlack)
	var window []inventory.Change
	for _, change := range v.changesOf(deployment) {
		if change.Created || change.At.Before(from) ||
			(!until.IsZero() && change.At.After(until)) {
			continue
		}
		window = append(window, change)
	}
	return revision, inventory.RevisionDiff(window)
}
