package investigate

import (
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// An investigation that ran before the incident had a cause confirms
// nothing; concluded again once the cause is known, what it read proves
// the cause (an OOM-killed run here).
func TestRecorrelateConfirmsACauseFoundAfterTheReads(t *testing.T) {
	model := failedRunModel(t, failedRuns[1].attrs)
	r := NewInvestigator(Sources{Model: model})
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	report := []incident.Section{{Check: incident.CheckPodState}}

	early := incidentOf(root, finding(crashContainer, "OOMKilled"))
	if c := r.Recorrelate(early, report); c != nil && c.Confirmed != "" {
		t.Fatalf("confirmed %q without a cause", c.Confirmed)
	}
	later := early
	later.Cause = &rootcause.CauseRecord{Root: root}
	c := r.Recorrelate(later, report)
	if c == nil || c.Confirmed == "" {
		t.Fatalf("correlation = %+v, want the OOM-killed run as proof", c)
	}
}
