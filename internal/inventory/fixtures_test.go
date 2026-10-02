package inventory

import (
	"testing"
	"time"
)

// testTime is a fixed clock start for history tests.
var testTime = time.Date(2026, 9, 29, 14, 2, 0, 0, time.UTC)

// historyModel is a model with a Deployment api that owns ReplicaSet
// api-1, which owns pod api-1-a; the Deployment references ConfigMap
// app-config.
type historyModel struct {
	*Model
	deployment, replicaSet, pod, config EntityID
}

func newHistoryModel(t testing.TB) historyModel {
	t.Helper()
	h := historyModel{
		Model:      NewModel(Options{}),
		deployment: CoreID("deployment", "shop", "api"),
		replicaSet: CoreID("replicaset", "shop", "api-1"),
		pod:        CoreID("pod", "shop", "api-1-a"),
		config:     CoreID("configmap", "shop", "app-config"),
	}
	for _, id := range []EntityID{h.deployment, h.replicaSet, h.pod,
		h.config} {
		h.apply(t, Observation{Kind: Observed, At: testTime, Entity: id})
	}
	h.relate(t, h.pod, OwnedBy, h.replicaSet)
	h.relate(t, h.replicaSet, OwnedBy, h.deployment)
	h.relate(t, h.deployment, References, h.config)
	return h
}

func (h historyModel) apply(t testing.TB, o Observation) {
	t.Helper()
	if o.Source == "" {
		o.Source = "test"
	}
	if _, err := h.Apply(o); err != nil {
		t.Fatal(err)
	}
}

func (h historyModel) relate(
	t testing.TB, from EntityID, relation RelationType, to EntityID,
) {
	t.Helper()
	h.apply(t, Observation{
		Kind: Related, At: testTime, Entity: from,
		Relation: relation, Targets: []EntityID{to},
	})
}

// change records a change of id at offset after testTime.
func (h historyModel) change(
	t testing.TB, id EntityID, offset time.Duration, change Change,
) {
	t.Helper()
	change.At = testTime.Add(offset)
	h.apply(t, Observation{
		Kind: Changed, At: change.At, Entity: id, Change: change,
	})
}

func imageChange(before, after string) Change {
	return Change{Actor: "argocd-controller", Fields: []FieldChange{{
		Path: "containers[api].image", Before: before, After: after,
	}}}
}
