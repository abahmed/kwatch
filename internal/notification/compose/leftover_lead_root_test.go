package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// groupAt is an incident of findings, each a leftover of root.
func groupAt(
	root inventory.EntityID, findings ...detection.Finding,
) incident.Incident {
	p := incident.Incident{ID: "group", Root: root,
		State: incident.Open, Tier: incident.Digest,
		Members: map[detection.Key]detection.Finding{}}
	for _, f := range findings {
		f.Leftover = root
		p.Members[f.Key()] = f
	}
	return p
}

// A group of a workload an autoscaler lost, without webhooks, says the
// controller was removed; the words come from the root.
func TestGoneControllerGroupIsTitledByIt(t *testing.T) {
	p := groupAt(inventory.CoreID("deployment", "mesh", "mesh-control"),
		leftoverFinding("horizontalpodautoscaler", "mesh",
			"mesh-control", reasons.HPATargetMissing),
		leftoverFinding("poddisruptionbudget", "mesh", "mesh-pdb",
			reasons.PdbSelectsNothing))

	got := NewWriter("", nil).ProblemTitle(p, nil, revisionNow)

	assert.Contains(t, got, "Leftovers of removed controller")
	assert.Contains(t, got, "mesh-control")
	assert.Contains(t, got, "2 objects")
	assert.NotContains(t, got, "mesh-pdb")
}

// Workloads of a namespace parked at zero replicas are listed by name
// under their namespace.
func TestParkedGroupListsItsWorkloads(t *testing.T) {
	var parked []detection.Finding
	for _, name := range []string{"cart", "search", "wishlist", "zoo"} {
		parked = append(parked, leftoverFinding("deployment", "demo",
			name, reasons.ScaledToZeroRouted))
	}
	p := groupAt(inventory.CoreID("namespace", "", "demo"), parked...)

	got := NewWriter("", nil).ProblemTitle(p, nil, revisionNow)

	assert.Contains(t, got, "Scaled to 0 but still routed in namespace "+
		"demo (4 workloads): cart, search, wishlist +1")
}
