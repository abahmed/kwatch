package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// HistoryChanges reads changes and their outcomes from the inventory's
// history. It implements ChangeReader and OutcomeReader.
type HistoryChanges struct {
	History inventory.HistoryReader
}

// Changes implements ChangeReader: the changes of id made between since
// and until. They come from the entity's own change ring, which feeds
// the change sets: reading the sets would scan every release for every
// candidate, and a storm has thousands of candidates.
func (h HistoryChanges) Changes(
	id inventory.EntityID, since, until time.Time,
) []inventory.Change {
	return ModelChanges{Reader: h.History}.Changes(id, since, until)
}

// Outcome implements OutcomeReader: the outcome, as of now, of the
// change set (the release) that holds change.
func (h HistoryChanges) Outcome(
	change inventory.Change, now time.Time,
) (inventory.Outcome, bool) {
	for _, set := range h.History.ChangeSets(change.Entity, change.At) {
		for _, member := range set.Changes {
			if member.Entity == change.Entity && member.At.Equal(change.At) {
				return h.History.OutcomeOf(set, now).Outcome, true
			}
		}
	}
	return "", false
}

// WorkloadBaselines judges a workload against its learned baselines:
// how long its pods usually wait to start and to become ready. It
// implements BaselineReader.
type WorkloadBaselines struct {
	History inventory.HistoryReader
	Now     time.Time
}

// Deviation implements BaselineReader. A workload with a pod waiting
// longer than its baseline allows is as unusual as it gets. Anything
// else, including a workload without enough history, is not judged:
// pending and ready times say nothing about a crash, so a normal
// reading is no evidence against a cause.
func (b WorkloadBaselines) Deviation(
	id inventory.EntityID,
) (float64, bool) {
	baselines := b.History.Baselines()
	for _, pod := range rootcause.OwnedPods(b.History, id) {
		metric, waited, ok := b.waiting(pod)
		if ok && baselines.IsUnusual(id, metric, waited.Seconds()) {
			return 1, true
		}
	}
	return 0, false
}

// waiting reports what a pod is waiting for and for how long: to
// start (pending time) or to become ready (ready time). ok is false
// for a ready pod or one without a creation time.
func (b WorkloadBaselines) waiting(
	pod inventory.EntityID,
) (inventory.Metric, time.Duration, bool) {
	entity, ok := b.History.Entity(pod)
	if !ok {
		return "", 0, false
	}
	created := attributeTime(entity, kube.AttrCreated)
	if created.IsZero() {
		return "", 0, false
	}
	waited := b.Now.Sub(created)
	if attributeTime(entity, kube.AttrStartTime).IsZero() {
		return inventory.MetricPendingSeconds, waited, true
	}
	if ready, _ := attributeValue(entity, kube.AttrReady).AsBool(); !ready {
		return inventory.MetricReadySeconds, waited, true
	}
	return "", 0, false
}

func attributeValue(e inventory.Entity, name string) inventory.Value {
	attribute, _ := e.Attribute(name)
	return attribute.Value
}

func attributeTime(e inventory.Entity, name string) time.Time {
	return attributeValue(e, name).AsTime()
}
