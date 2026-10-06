package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// poolOf builds a pool whose nodes were created the given ages before
// now.
func poolOf(
	t *testing.T, now time.Time, ages ...time.Duration,
) (*inventory.Model, inventory.EntityID) {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	pool := inventory.CoreID(KindNodePool, "", "workers")
	apply := func(o inventory.Observation) {
		_, err := model.Apply(o)
		require.NoError(t, err)
	}
	for i, age := range ages {
		node := inventory.CoreID(KindNode, "", string(rune('a'+i)))
		apply(inventory.Observation{
			Kind: inventory.Observed, Source: ObservationSource, At: now,
			Entity: node, Attributes: map[string]inventory.Value{
				AttrCreated: inventory.Time(now.Add(-age))},
		})
		apply(inventory.Observation{
			Kind: inventory.Related, Source: ObservationSource, At: now,
			Entity: node, Relation: inventory.PartOf,
			Targets: []inventory.EntityID{pool},
		})
	}
	return model, pool
}

func TestGroupBootRemaining(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := map[string]struct {
		ages []time.Duration
		want time.Duration
	}{
		"scaled up from zero": {
			[]time.Duration{time.Minute, time.Minute, 2 * time.Minute},
			9 * time.Minute},
		"one replacement in an old pool": {
			[]time.Duration{time.Minute, day, day}, 0},
		"half the pool is new": {
			[]time.Duration{time.Minute, day}, 9 * time.Minute},
		"boot is over": {
			[]time.Duration{11 * time.Minute, 12 * time.Minute}, 0},
		"no nodes": {nil, 0},
	}
	for name, c := range cases {
		model, pool := poolOf(t, now, c.ages...)
		assert.Equal(t, c.want, GroupBootRemaining(model, pool, now), name)
	}
}

func TestGroupBootRemainingIgnoresAnUnsetClock(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	model, pool := poolOf(t, now, time.Minute)

	assert.Zero(t, GroupBootRemaining(model, pool, time.Time{}))
}

func TestNodeYoungRemainingLooksAtTheNodeAlone(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	model, _ := poolOf(t, now, time.Minute, 3*time.Hour)
	young := inventory.CoreID(KindNode, "", "a")
	old := inventory.CoreID(KindNode, "", "b")

	assert.Equal(t, 9*time.Minute, NodeYoungRemaining(model, young, now))
	assert.Zero(t, NodeYoungRemaining(model, old, now),
		"an old node in a booting pool is not young")
	assert.Zero(t, NodeYoungRemaining(model,
		inventory.CoreID(KindNode, "", "gone"), now))
}
