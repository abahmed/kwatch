package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A field that was not set before says what it became: "The
// pod-security changed." alone only repeats the lead's "after the change
// to its pod-security".
func TestChangeOfAnUnsetFieldSaysWhatItBecame(t *testing.T) {
	ns := inventory.CoreID(kube.KindNamespace, "", "reports")
	rs := inventory.CoreID(kube.KindReplicaSet, "reports", "api-4f8b")
	change := &inventory.Change{Entity: ns, At: at(0, 0),
		Fields: []inventory.FieldChange{{Path: "labels.pod-security",
			After: "enforce=restricted"}}}
	p := incident.Incident{ID: "inc-1", Root: ns, Tier: incident.Notify,
		State: incident.Open, Opened: at(1, 0), Revision: 1,
		Members: members(detection.Finding{Entity: rs, Reason: "FailedCreate",
			Severity: detection.Critical, Since: at(1, 0),
			Summary: "ReplicaSet cannot create pods"}),
		Cause: &rootcause.CauseRecord{Rule: "pod-security-rejects",
			Root: ns, Score: 0.8, Change: change}}

	got := Writer{}.Write(announce(p), at(6, 0)).Note

	assert.Contains(t, got, "The pod-security changed to "+
		"enforce=restricted.")
}
