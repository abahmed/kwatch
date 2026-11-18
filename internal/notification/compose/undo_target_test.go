package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The rollback revision is the blamed workload's, so a rollback of the
// root goes to it only when the blamed change is the root's own. A
// cause rooted at payments that blames a change of checkout offers no
// rollback of payments, as the dashboard offers none for a release the
// cause does not blame.
func TestRollbackOnlyOfTheBlamedWorkload(t *testing.T) {
	p := badRollout()
	own := Writer{}.Write(announce(p), at(5, 0))
	assert.True(t, hasUndo(own.Steps), "the root's own release")

	p = badRollout()
	p.Cause.Change.Entity = inventory.CoreID(kube.KindDeployment,
		"shop", "checkout")
	other := Writer{}.Write(announce(p), at(5, 0))
	assertNoUndo(t, other.Steps)
	assert.NotContains(t, other.Note, "rollout undo")
	assert.NotContains(t, stepTexts(other.Steps), "revision 13")
}
