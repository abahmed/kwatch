package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A change that records the end of a link to the failing workload says
// so, even though the model no longer links the two; one to an object
// the failing pods have nothing to do with gets no sentence of its own.
func TestChangeWhyNamesAnEndedLinkToTheFailingWorkload(t *testing.T) {
	tm, p, ids := reasonsFixture(t)
	links := newChangeLinks(tm.m, p)
	gone := inventory.CoreID(kube.KindService, "shop", "payments")
	ended := func(to inventory.EntityID) inventory.Change {
		return inventory.Change{Entity: gone, At: testEpoch,
			Fields: []inventory.FieldChange{
				inventory.LinkRemoved(inventory.RoutesTo, to)}}
	}
	assert.Equal(t, "This Service stopped routing to the workload the "+
		"failing pods belong to.", links.why(ended(ids["deployment"])))
	assert.Equal(t, "stopped routing to deployment api",
		changeSummary(ended(ids["deployment"])))
	assert.Empty(t, links.why(ended(
		inventory.CoreID(kube.KindDeployment, "shop", "web"))))
}
