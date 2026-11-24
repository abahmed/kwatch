package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A workload blamed on its rollout that is now only scaled to zero has
// nothing the rollout explains: neither the lead nor a step blames or
// reverts the change any more.
func TestScaledToZeroDropsTheStaleRolloutUndo(t *testing.T) {
	delta := inventory.CoreID(kube.KindDeployment, "apps", "delta")
	p := incident.Incident{ID: "d-1", Root: delta, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Cause: &rootcause.CauseRecord{Rule: "rollout", Root: delta,
			Score: 0.95, RollbackRevision: "4", PreviousServed: true,
			Change: &inventory.Change{Entity: delta, App: "argocd/delta",
				At: writerNow.Add(-2 * time.Hour), Revision: "5",
				Fields: []inventory.FieldChange{{
					Path: "containers[app].image", After: "delta:2"}}}},
		Members: members(detection.Finding{Entity: delta,
			Reason: reasons.ScaledToZeroRouted, Severity: detection.Warning,
			Since:   writerNow.Add(-time.Hour),
			Summary: "Service delta routes to a workload scaled to 0"}),
	}
	msg := Writer{}.Write(announce(p), writerNow)
	for _, s := range msg.Steps {
		assert.NotContains(t, s.Text, "Revert the change")
		assert.NotContains(t, s.Command, "rollout undo")
	}
	assert.False(t, strings.Contains(msg.Title, "change"), msg.Title)
}
