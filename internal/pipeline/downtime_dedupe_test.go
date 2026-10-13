package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

func downtimeTemplateEdit(snapshot time.Time) inventory.Observation {
	return inventory.Observation{Kind: inventory.Changed,
		Entity: restartDeploy, At: snapshot.Add(time.Hour),
		Change: inventory.Change{At: snapshot, Actor: DowntimeActor,
			Fields: []inventory.FieldChange{{Path: "spec.template"}}}}
}

// A real change restored from the previous run, made after the last
// snapshot, replaces the downtime guess for the same entity and class.
func TestRestoredChangeWinsOverDowntimeGuess(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	snapshot := restartBase.Add(-20 * time.Minute)
	model.RestoreChanges([]inventory.Change{
		templateChange("payments", snapshot.Add(5*time.Minute))})
	guess := downtimeTemplateEdit(snapshot)

	kept := withoutRestored(model, []inventory.Observation{guess})

	assert.Empty(t, kept)
}

// A restored change from before the snapshot, or of another class, does
// not explain the difference the fingerprints found.
func TestRestoredChangeDoesNotHideAnotherEdit(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	snapshot := restartBase.Add(-20 * time.Minute)
	model.RestoreChanges([]inventory.Change{
		templateChange("payments", snapshot.Add(-time.Hour)),
		{Entity: restartDeploy, At: snapshot.Add(time.Minute),
			Fields: []inventory.FieldChange{{Path: "spec.replicas"}}},
	})
	guess := downtimeTemplateEdit(snapshot)

	kept := withoutRestored(model, []inventory.Observation{guess})

	assert.Len(t, kept, 1)
}
