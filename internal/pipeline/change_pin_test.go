package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

const pinImage = "spec.template.spec.containers[app].image"

// rollout is bob's image change of the payments deployment at at.
func rollout(at time.Time, before, after string) inventory.Change {
	return inventory.Change{Entity: restartDeploy, At: at, Actor: "bob",
		Revision: "8", Fields: []inventory.FieldChange{
			{Path: pinImage, Before: before, After: after}}}
}

// openIncidentOn is the record of an incident on the payments
// deployment that opened a minute after change and blames it.
func openIncidentOn(change inventory.Change) incident.Record {
	return incident.Record{ID: "inc-pinned", Root: restartDeploy,
		Tier: incident.Notify, State: incident.Open,
		Opened:    change.At.Add(time.Minute),
		Announced: change.At.Add(2 * time.Minute),
		Cause: &rootcause.CauseRecord{Rule: "rollout",
			Root: restartDeploy, Change: &change}}
}

// restartWith saves records and changes, then starts a new run on the
// same store as a restart does: incidents first, then the history.
func restartWith(t *testing.T, records []incident.Record,
	changes ...inventory.Change,
) *harness {
	t.Helper()
	store := NewIncidentStore(openTempStore(t))
	saved := make([]changeRecord, 0, len(changes))
	for _, change := range changes {
		saved = append(saved, changeRecord{Change: change})
	}
	require.NoError(t, store.(changeStore).AppendChanges(saved))
	require.NoError(t, store.SaveIncidents(records))
	h := newHarnessWith(t, restartBase, func(d *Dependencies) {
		d.Store = store
	})
	require.NoError(t, h.engine.restore())
	h.engine.startHistory()
	t.Cleanup(func() { h.engine.storage.history.askStop() })
	return h
}

// An incident still open three days after the rollout it blames keeps
// that rollout across a restart, and the rollback that ends it is
// credited as a rollback. Its resolve unpins the old rollout.
func TestRestartPinsOldChangeOfOpenIncident(t *testing.T) {
	change := rollout(restartBase.Add(-72*time.Hour), "app:2.2", "app:2.3")
	h := restartWith(t, []incident.Record{openIncidentOn(change)}, change)
	model := h.engine.deps.Model

	kept := model.Changes(restartDeploy, time.Time{})
	require.Len(t, kept, 1, "the blamed rollout is pinned")
	assert.Equal(t, "bob", kept[0].Actor)
	assert.NotEmpty(t, model.ChangeSets(restartDeploy, change.At),
		"the rollout's change set links it")

	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("payments")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 2, 2
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, pod("payments-1", rs.Name, "n1", true, h.now))
	back := rollout(h.now.Add(30*time.Second), "app:2.3", "app:2.2")
	_, err := model.Apply(inventory.Observation{Kind: inventory.Changed,
		Source: "test", At: back.At, Entity: restartDeploy, Change: back})
	require.NoError(t, err)

	var resolved *incident.Decision
	done := func() bool {
		for i := range h.decisions {
			if h.decisions[i].Action == incident.Resolve {
				resolved = &h.decisions[i]
				return true
			}
		}
		return false
	}
	require.True(t, h.runUntil(done, h.now.Add(2*time.Hour),
		10*time.Second), "the incident resolves")
	assert.Equal(t, incident.FixRollback, resolved.Incident.Fix)
	for _, c := range model.Changes(restartDeploy, time.Time{}) {
		assert.False(t, c.At.Equal(change.At), "the resolve unpins it")
	}
}

// Pinning stops at maxPinnedPerEntity per entity and at
// inventory.DefaultMaxPinnedChanges in all, keeping the newest, and
// leaves other namespaces alone.
func TestRestartPinningIsBounded(t *testing.T) {
	first := restartBase.Add(-6 * 24 * time.Hour)
	blamed := rollout(first, "app:2.2", "app:2.3")
	changes := []inventory.Change{blamed}
	entities := inventory.DefaultMaxPinnedChanges/maxPinnedPerEntity + 2
	for e := range entities {
		id := kube.EntityFor("apps/v1", "Deployment", "shop",
			fmt.Sprintf("w%02d", e))
		for i := range maxPinnedPerEntity + 3 {
			at := first.Add(time.Duration(e*100+i+1) * time.Minute)
			changes = append(changes, inventory.Change{Entity: id,
				At: at, Fields: []inventory.FieldChange{{Path: "x"}}})
		}
	}
	other := kube.EntityFor("apps/v1", "Deployment", "other", "api")
	changes = append(changes, inventory.Change{Entity: other,
		At: first.Add(time.Hour), Fields: []inventory.FieldChange{
			{Path: "x"}}})
	h := restartWith(t, []incident.Record{openIncidentOn(blamed)},
		changes...)

	pinned := h.engine.storage.history.pinnedChanges(
		restartBase.Add(-inventory.ChangeRetention))
	assert.Len(t, pinned, inventory.DefaultMaxPinnedChanges)
	newest := kube.EntityFor("apps/v1", "Deployment", "shop",
		fmt.Sprintf("w%02d", entities-1))
	model := h.engine.deps.Model
	assert.Len(t, model.Changes(newest, time.Time{}), maxPinnedPerEntity)
	assert.Empty(t, model.Changes(other, time.Time{}))
	assert.Empty(t, model.Changes(restartDeploy, time.Time{}),
		"the oldest change is past the overall cap")
}

// Without an open incident a restart pins nothing.
func TestRestartWithoutOpenIncidentPinsNothing(t *testing.T) {
	change := rollout(restartBase.Add(-72*time.Hour), "app:2.2", "app:2.3")
	resolved := openIncidentOn(change)
	resolved.State, resolved.Resolved = incident.Resolved,
		change.At.Add(time.Hour)
	for _, records := range [][]incident.Record{nil, {resolved}} {
		h := restartWith(t, records, change)
		assert.Empty(t, h.engine.deps.Model.Changes(restartDeploy,
			time.Time{}))
		assert.Empty(t, h.engine.storage.history.pinnedBy)
	}
}
