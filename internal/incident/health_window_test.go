package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func windowFixture(t *testing.T) (*inventory.Model, inventory.EntityID,
	time.Time) {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	id := kube.EntityFor("apps/v1", "Deployment", "shop", "api")
	now := time.Date(2026, 10, 13, 10, 5, 0, 0, time.UTC)
	m.RestoreChanges([]inventory.Change{{
		Entity: id, At: now.Add(-3 * time.Minute), Actor: "bob",
		Fields: []inventory.FieldChange{{Path: "spec.template"}},
	}})
	return m, id, now
}

func windowIncident(id inventory.EntityID, failed time.Time) Incident {
	// Without member findings the incident's opening is the failure.
	return Incident{Root: id, Opened: failed}
}

func TestWindowOfNeedsAMarkAndAChange(t *testing.T) {
	m, id, now := windowFixture(t)
	failed := now.Add(-2 * time.Minute)
	p := windowIncident(id, failed)

	assert.Nil(t, WindowOf(m, p, now), "no healthy mark, no window")

	m.RestoreHealth(id, inventory.HealthMark{
		At: now.Add(-5 * time.Minute), Revision: "7"})
	w := WindowOf(m, p, now)
	require.NotNil(t, w)
	assert.True(t, w.Healthy.Equal(now.Add(-5*time.Minute)))
	assert.True(t, w.Failed.Equal(failed))
	assert.Equal(t, "7", w.Revision)
	require.Len(t, w.Changes, 1)
	assert.Equal(t, "bob", w.Changes[0].Actor)
}

func TestWindowOfIgnoresMarksAfterTheFailureAndOldOnes(t *testing.T) {
	m, id, now := windowFixture(t)
	failed := now.Add(-2 * time.Minute)
	p := windowIncident(id, failed)
	m.RestoreHealth(id, inventory.HealthMark{At: now.Add(-time.Minute)})
	assert.Nil(t, WindowOf(m, p, now), "a mark after the failure")

	old := now.Add(-WindowHorizon - time.Hour)
	m2 := inventory.NewModel(inventory.Options{})
	m2.RestoreHealth(id, inventory.HealthMark{At: old})
	assert.Nil(t, WindowOf(m2, p, now), "beyond the change history")
}

func TestWindowOfLeavesOutTheBlamedChange(t *testing.T) {
	m, id, now := windowFixture(t)
	m.RestoreHealth(id, inventory.HealthMark{At: now.Add(-5 * time.Minute)})
	p := windowIncident(id, now.Add(-2*time.Minute))
	blamed := m.Changes(id, time.Time{})[0]
	p.Cause = &rootcause.CauseRecord{Root: id, Change: &blamed}

	assert.Nil(t, WindowOf(m, p, now), "the cause is already said")
}
