package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func lostNodeFinding(name string) detection.Finding {
	return detection.Finding{
		Entity: inventory.CoreID(kube.KindNode, "", name),
		Reason: reasons.NodeNotReady, Mode: detection.ModeNotReady,
		Severity: detection.Critical, Since: at(0, 0),
		Summary: "Node has not reported for about two minutes",
		Evidence: []detection.Evidence{{Label: "error",
			Value: "Kubelet stopped posting node status"},
			{Label: detection.EvidenceReadyStatus, Value: "Unknown"}},
	}
}

// groupedOutage is one outage of several nodes that failed in the same
// settle window: the first is the root, the rest joined it.
func groupedOutage(names ...string) incident.Incident {
	var found []detection.Finding
	var merged []inventory.EntityID
	for i, name := range names {
		found = append(found, lostNodeFinding(name))
		if i > 0 {
			merged = append(merged, found[i].Entity)
		}
	}
	return incident.Incident{ID: "inc-1", Root: found[0].Entity,
		Tier: incident.Page, State: incident.Open, Opened: at(0, 0),
		Revision: 1, Merged: merged, Members: members(found...)}
}

func TestGroupedNodeOutageSpeaksOfAllNodesInThePlural(t *testing.T) {
	p := groupedOutage("n1", "n2", "n3")

	note := Writer{Cluster: "prod-eu-1"}.Write(announce(p), at(6, 0)).Note

	assert.Contains(t, note, "Three nodes (prod-eu-1) have not reported "+
		"for about two minutes: n1, n2 and n3.")
	assert.Contains(t, note, "They report \"Kubelet stopped posting node "+
		"status\".")
	assert.Contains(t, note, "the kubelets stopped reporting, so the "+
		"nodes may be down or unreachable.")
	assert.NotContains(t, note, "It reports")
	assert.NotContains(t, note, "so the node may")
}

func TestLoneNodeKeepsTheSingularProof(t *testing.T) {
	p := groupedOutage("n1")

	note := Writer{}.Write(announce(p), at(6, 0)).Note

	assert.Contains(t, note, "It reports \"Kubelet stopped posting node "+
		"status\".")
	assert.Contains(t, note, "so the node may be down or unreachable.")
}

func spreadPod(app, suffix string) inventory.EntityID {
	return inventory.CoreID(kube.KindPod, "shop", app+"-7d9f-"+suffix)
}

func podsNotReady(apps ...string) ([]detection.Finding, []incident.Event,
	[]inventory.EntityID,
) {
	var found []detection.Finding
	var events []incident.Event
	var impact []inventory.EntityID
	for _, app := range apps {
		pod := spreadPod(app, "a")
		found = append(found, detection.Finding{Entity: pod,
			Reason: reasons.ContainersNotReady, Mode: detection.ModeNotReady,
			Severity: detection.Warning, Since: at(3, 0),
			Summary: "Pod has not been ready for three minutes"})
		events = append(events, incident.Event{At: at(3, 0),
			Text:   "Pod has not been ready for three minutes",
			Entity: aboutEntity(pod)})
		impact = append(impact,
			inventory.CoreID(kube.KindDeployment, "shop", app))
	}
	return found, events, impact
}

func TestGroupedNodeOutageUpdateNamesTheNodesOnce(t *testing.T) {
	p := groupedOutage("n1", "n2", "n3", "n4", "n5")
	found, events, impact := podsNotReady("auth", "cart")
	for _, f := range found {
		p.Members[f.Key()] = f
	}
	p.Timeline, p.Impact, p.Revision = events, impact, 2
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}

	note := Writer{Cluster: "prod-eu-1"}.Write(d, at(6, 0)).Note

	assert.Contains(t, note, "Nodes n1–n5 (prod-eu-1) are spreading to "+
		"auth and cart in shop: they have not been ready for three "+
		"minutes.")
	assert.NotContains(t, note, "Node n1")
}

// A zone that fails as a whole is the subject of its own update, and
// the node that happens to be the worst member is never "spreading" to
// itself.
func TestZoneUpdateSpeaksForTheZoneNotItsWorstNode(t *testing.T) {
	zone := inventory.CoreID(kube.KindZone, "", "zone-a")
	a1 := lostNodeFinding("a1")
	found, events, impact := podsNotReady("auth", "cart")
	events = append([]incident.Event{{At: at(3, 0),
		Text: "Node has not reported", Entity: aboutEntity(a1.Entity)}},
		events...)
	p := incident.Incident{ID: "inc-1", Root: zone, Tier: incident.Page,
		State: incident.Open, Opened: at(0, 0), Revision: 2,
		Members:  members(append(found, a1, lostNodeFinding("a2"))...),
		Timeline: events, Impact: impact,
		Cause: &rootcause.CauseRecord{Rule: "zone-failing", Root: zone}}
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}

	note := Writer{Cluster: "prod-eu-1"}.Write(d, at(6, 0)).Note

	assert.Contains(t, note, "Zone zone-a (prod-eu-1) is spreading to "+
		"auth and cart in shop")
	assert.NotContains(t, note, "spreading to node a1")
	assert.NotContains(t, note, "Node a1 (prod-eu-1) is spreading")
}
