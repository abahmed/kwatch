package compose

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// An incident caused by a zone or node pool leads with the place and
// its nodes, not with whichever pod happens to be the worst member.
func TestLeadForGroupCauseNamesTheGroupAndItsNodes(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	pool := inventory.CoreID(kube.KindNodePool, "", "dev-arm-np")
	n1 := inventory.CoreID(kube.KindNode, "", "n1")
	n2 := inventory.CoreID(kube.KindNode, "", "n2")
	pod := inventory.CoreID(kube.KindPod, "datadog", "datadog-agent-zb9s4")
	members := map[detection.Key]detection.Finding{
		{Entity: n1, Reason: "NodeNotReady"}: {Entity: n1,
			Reason: "NodeNotReady", Severity: detection.Warning,
			Summary: "Node has not reported for about two minutes"},
		{Entity: n2, Reason: "NodeNotReady"}: {Entity: n2,
			Reason: "NodeNotReady", Severity: detection.Warning,
			Summary: "Node has not reported for about two minutes"},
		{Entity: pod, Reason: "ContainersNotReady"}: {Entity: pod,
			Reason: "ContainersNotReady", Severity: detection.Critical,
			Summary: "Pod has not been ready for three minutes"},
	}
	p := incident.Incident{
		ID: "inc-1", Root: pool, Tier: incident.Page, State: incident.Open,
		Opened: now, Announced: now, Revision: 1, Members: members,
		Cause: &rootcause.CauseRecord{Root: pool, Rule: "nodepool-failing"},
	}

	msg := Writer{}.Write(incident.Decision{Action: incident.Announce,
		Incident: p}, now)

	want := "Node pool dev-arm-np is failing as a whole: two nodes have " +
		"not reported for about two minutes."
	if msg.Title != want {
		t.Fatalf("title = %q\nwant    %q", msg.Title, want)
	}
}
