package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ownChangeRig opens an incident on a workload blamed on its own
// change, as an earlier version of the reasoning stated it.
func ownChangeRig(t *testing.T) (*rig, detection.Finding) {
	r := newRig(t, Config{})
	deploy := entity(kube.KindDeployment, "web")
	held := sig(deploy, "ScaledToZeroRouted", detection.Warning)
	c := r.cause(deploy, deploy, "deployment web changed")
	c.Row = "own-change"
	c.Changes = []inventory.Change{{Entity: deploy, At: t0}}
	r.rule.causes[deploy] = c
	r.raise(at(0), held)
	r.tick(at(DefaultSettle))
	require.NotNil(t, r.only().Cause)
	require.NotNil(t, r.only().Cause.Change)
	r.m.SetSteps(r.only().ID, []FixStep{{Text: "Revert the change"}})
	r.m.TakeUntitled()
	delete(r.rule.causes, deploy)
	return r, held
}

// A solve that refutes the root's own change drops the change as the
// cause: a workload held at zero is not broken by its image edit.
func TestRefutedOwnChangeIsDropped(t *testing.T) {
	r, held := ownChangeRig(t)
	r.rule.refuted = map[inventory.EntityID]bool{held.Entity: true}

	r.apply(at(5*time.Minute), detection.Changed, held)

	assert.Nil(t, r.only().Cause)
	assert.True(t, r.only().StepsStale(), "steps follow the cause")
	assert.Len(t, r.m.TakeUntitled(), 1, "the title follows the cause")
}

// A solve that merely states no cause (its evidence aged out) keeps
// the change: only a refutation drops it.
func TestUnrefutedOwnChangeIsKept(t *testing.T) {
	r, held := ownChangeRig(t)

	r.apply(at(5*time.Minute), detection.Changed, held)

	assert.NotNil(t, r.only().Cause)
	assert.False(t, r.only().StepsStale())
}
