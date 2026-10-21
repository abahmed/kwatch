package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The same workloads failing in two ways are named once, with both
// states, not in two sentences that repeat the names.
func TestUpdateNamesWorkloadsOnceWhenTheyFailInTwoWays(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n3")
	var found []detection.Finding
	var events []incident.Event
	var impact []inventory.EntityID
	for _, app := range []string{"fleet", "storage"} {
		dep := inventory.CoreID(kube.KindDeployment, "shop", app)
		pod := inventory.CoreID(kube.KindPod, "shop", app+"-7d9f-a")
		impact = append(impact, dep)
		for _, c := range []struct {
			id      inventory.EntityID
			reason  string
			summary string
		}{
			{dep, "DeploymentUnavailable", "0 of 1 replicas are ready"},
			{pod, reasons.PodPending, "Pod has been Pending since it " +
				"was scheduled"},
		} {
			found = append(found, detection.Finding{Entity: c.id,
				Reason: c.reason, Severity: detection.Warning,
				Since: at(3, 0), Summary: c.summary})
			events = append(events, incident.Event{At: at(3, 0),
				Text: c.summary, Entity: aboutEntity(c.id)})
		}
	}
	found = append(found, lostNodeFinding("n3"))
	p := incident.Incident{ID: "inc-1", Root: node, Tier: incident.Page,
		State: incident.Open, Opened: at(0, 0), Revision: 2,
		Members: members(found...), Timeline: events, Impact: impact}
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}

	note := Writer{}.Write(d, at(6, 0)).Note

	assert.Equal(t, 1, strings.Count(note, "fleet"), note)
	assert.NotContains(t, note, "also spreading")
}
