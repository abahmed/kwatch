package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A rival that only says the subject is failing is the subject's own
// state again: the possible cause is told as what the subject lacks.
func TestRivalLeadDoesNotRestateTheSubjectAsACause(t *testing.T) {
	claim := inventory.CoreID("resourceclaim", "ml", "gpu-claim")
	sched := inventory.CoreID("scheduling", "", "cannot allocate all claims")
	unallocated := detection.Finding{Entity: claim,
		Reason: "ResourceClaimUnallocated", Severity: detection.Warning,
		Since: at(0, 0), Summary: "ResourceClaim is failing"}
	p := incident.Incident{ID: "inc-1", Root: claim, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 2,
		Members: members(unallocated),
		Cause: &rootcause.CauseRecord{Rule: "resource-claim-unallocated",
			Root: claim, Score: 0.69, RootFindings: []detection.Finding{
				unallocated},
			Rival: &rootcause.CauseRecord{Rule: "scheduler-capacity",
				Root: sched, Score: 0.85}}}

	got := Writer{}.Write(announce(p), at(6, 0)).Title

	assert.Equal(t, 1, strings.Count(got, "gpu-claim"), got)
	assert.Contains(t, got, "two possible causes: it has no device "+
		"allocated, so its pod cannot start or the scheduler rejects "+
		"every node")
}
