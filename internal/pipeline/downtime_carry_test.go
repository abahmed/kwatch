package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func savedFingerprints(
	t *testing.T, m *inventory.Model, at time.Time,
) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range Fingerprints(m, at) {
		out[k] = v.(string)
	}
	return out
}

func savedPrints(e *Engine) map[string]any {
	return e.storage.incidents.pending.fingerprints
}

// A kind that has not synced when the downtime comparison runs is not
// compared, and its stored baseline is saved back unchanged; once the
// kind syncs, its changes are found and dated at the old snapshot.
func TestEngineCarriesFingerprintsOfUnsyncedKinds(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	snapshot := now.Add(-time.Hour)
	saved := savedFingerprints(t, modelWithDeployment(t, 2), snapshot)
	model := modelWithDeployment(t, 5)
	id := model.Entities(kube.KindDeployment)[0]
	synced := false
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink,
		func(d *Dependencies) {
			d.Model, d.Store = model, &memStore{}
			d.Synced = func(k inventory.Kind) bool {
				return k != kube.KindDeployment || synced
			}
		})
	e.storage.saved = saved

	e.reconcileDowntime()
	e.save()

	if got := model.Changes(id, time.Time{}); len(got) != 0 {
		t.Fatalf("unsynced kind was compared: %+v", got)
	}
	prints := savedPrints(e)
	if prints[id.String()] != saved[id.String()] {
		t.Errorf("saved %v, want the stored baseline %v",
			prints[id.String()], saved[id.String()])
	}
	timeKey := kindTimePrefix + string(kube.KindDeployment)
	if prints[timeKey] != saved[snapshotTimeKey] {
		t.Errorf("carried snapshot time = %v", prints[timeKey])
	}

	synced = true
	e.step(context.Background(), now, newRechecks())
	e.save()

	changes := model.Changes(id, time.Time{})
	if len(changes) != 1 || changes[0].Actor != DowntimeActor ||
		!changes[0].At.Equal(snapshot) {
		t.Fatalf("changes = %+v, want one dated %v", changes, snapshot)
	}
	prints = savedPrints(e)
	if prints[timeKey] == saved[snapshotTimeKey] ||
		prints[id.String()] == saved[id.String()] {
		t.Errorf("synced kind still carried: %v", prints)
	}
}

// A baseline carried over a restart keeps its own snapshot time, so its
// changes are not dated at the newer snapshot of the other kinds.
func TestDowntimeChangesDatesCarriedKindAtItsSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	old := now.Add(-3 * time.Hour)
	saved := savedFingerprints(t, modelWithDeployment(t, 2),
		now.Add(-time.Hour))
	saved[kindTimePrefix+string(kube.KindDeployment)] =
		old.Format(time.RFC3339)

	compare, carried := splitSaved(saved,
		func(inventory.Kind) bool { return true })
	observations := DowntimeChanges(modelWithDeployment(t, 5), compare, nil,
		now)

	if len(carried) != 0 || len(observations) != 1 ||
		!observations[0].Change.At.Equal(old) {
		t.Fatalf("carried = %v observations = %+v", carried, observations)
	}
}

func TestSplitSavedCarriesOnlyUnsyncedKinds(t *testing.T) {
	deploy := inventory.NewEntityID("", kube.KindDeployment, "shop", "a")
	secret := inventory.NewEntityID("", kube.KindSecret, "shop", "s")
	saved := map[string]string{
		snapshotTimeKey:        "t1",
		deploy.String():        "replicas=1",
		secret.String():        "digest=x",
		"not an entity at all": "kept",
	}
	compare, carried := splitSaved(saved, func(k inventory.Kind) bool {
		return k != kube.KindSecret
	})

	if _, ok := compare[secret.String()]; ok || len(compare) != 3 {
		t.Errorf("compare = %v", compare)
	}
	kept := carried[kube.KindSecret]
	if len(carried) != 1 || kept.at != "t1" ||
		kept.entries[secret.String()] != "digest=x" {
		t.Errorf("carried = %+v", carried)
	}
	never := func(inventory.Kind) bool { return false }
	if late := carried.due(never); late != nil {
		t.Errorf("unsynced kind due: %v", late)
	}
	compare, carried = splitSaved(saved, nil)
	if len(compare) != len(saved) || len(carried) != 0 {
		t.Errorf("nil synced must compare all: %v %v", compare, carried)
	}
}
