package inventory

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scaleChange is an autoscaler's replica edit of the workload.
func scaleChange(id EntityID, i int) Observation {
	return Observation{Kind: Changed, Source: "test", Entity: id,
		At: ringBase.Add(time.Duration(i+1) * time.Second),
		Change: Change{Fields: []FieldChange{{Path: "spec.replicas",
			Before: strconv.Itoa(i%5 + 1), After: strconv.Itoa(i%5 + 2)}}}}
}

func hasImageChange(changes []Change) bool {
	for _, c := range changes {
		for _, f := range c.Fields {
			if f.Path == "spec.image" {
				return true
			}
		}
	}
	return false
}

// An autoscaled workload's replica edits fill neither its own change
// history nor the cluster's: its rollout stays readable after hundreds
// of them, and so do the latest replica edits.
func TestScaleOnlyChangesDoNotEvictARollout(t *testing.T) {
	m := NewModel(Options{})
	app := CoreID("deployment", "shop", "web")
	other := CoreID("deployment", "shop", "api")
	_, err := m.Apply(Observation{Kind: Changed, Source: "test",
		Entity: app, At: ringBase, Change: Change{Fields: []FieldChange{
			{Path: "spec.image", Before: "v1", After: "v2"}}}})
	require.NoError(t, err)

	for i := range 2 * DefaultMaxRecentChanges {
		_, _ = m.Apply(scaleChange(app, i))
		_, _ = m.Apply(scaleChange(other, i))
	}

	changes := m.Changes(app, ringBase)
	assert.True(t, hasImageChange(changes),
		"the rollout stays in the workload's history")
	assert.Len(t, changes, 1+DefaultMaxChangesPerEntity)
	last := changes[len(changes)-1]
	assert.True(t, ScaleOnly(last), "the latest replica edit is kept")
	var sets []Change
	for _, set := range m.RecentChangeSets(ringBase) {
		sets = append(sets, set.Changes...)
	}
	assert.True(t, hasImageChange(sets),
		"the rollout stays in the cluster history")
}

func TestScaleOnlyNamesOnlyReplicaEdits(t *testing.T) {
	replicas := FieldChange{Path: "spec.replicas", Before: "1", After: "0"}
	image := FieldChange{Path: "spec.image", Before: "a", After: "b"}
	assert.True(t, ScaleOnly(Change{Fields: []FieldChange{replicas}}))
	assert.False(t, ScaleOnly(Change{Fields: []FieldChange{
		replicas, image}}))
	assert.False(t, ScaleOnly(Change{Created: true}))
	assert.False(t, ScaleOnly(Change{}))
}
