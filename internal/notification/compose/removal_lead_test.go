package compose

import (
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

var removedN3 = inventory.CoreID(kube.KindNode, "", "n3")

// removalIncident is a page whose blamed change removed node n3 at
// 09:50 UTC, with the given members.
func removalIncident(change inventory.Change,
	found ...detection.Finding) incident.Incident {
	return incident.Incident{
		ID: "removal-1", Root: removedN3, Tier: incident.Page,
		State: incident.Open, Opened: writerNow.Add(-5 * time.Minute),
		Members: members(found...),
		Cause: &rootcause.CauseRecord{Root: removedN3, Score: 0.9,
			Change: &change},
	}
}

func n3Removed(actor string) inventory.Change {
	return inventory.Change{Entity: removedN3, Deleted: true,
		Actor: actor, At: writerNow.Add(-10 * time.Minute)}
}

func unavailable(name string) detection.Finding {
	return detection.Finding{
		Entity: inventory.CoreID(kube.KindDeployment, "shop", name),
		Reason: reasons.DeploymentUnavailable, Severity: detection.Critical,
		Since: writerNow.Add(-5 * time.Minute), Summary: "0 of 1 replicas are ready"}
}

// The workloads a node's removal broke lead, and the removal is why.
func TestRemovalLeadNamesTheWorkloadsAndTheRemoval(t *testing.T) {
	pending := detection.Finding{
		Entity: inventory.CoreID(kube.KindPod, "shop", "web-7d9f-a"),
		Reason: reasons.Unschedulable, Mode: detection.ModeUnschedulable,
		Severity: detection.Warning, Since: writerNow.Add(-5 * time.Minute),
		Summary: "Pod cannot be scheduled"}
	p := removalIncident(n3Removed("cluster-autoscaler"),
		unavailable("api"), unavailable("web"), pending)

	msg := Writer{}.Write(announce(p), writerNow)

	assert.Contains(t, msg.Note, "api and web in shop have no ready "+
		"replicas (0 of 1) because node n3 was removed at 09:50 UTC by "+
		"cluster-autoscaler and no other node has room.")
	assert.NotContains(t, msg.Note, "is down")
	assert.NotContains(t, msg.Note, "UTC change")
}

// Without a scheduling failure the lead does not say there is no room.
func TestRemovalLeadSaysNoRoomOnlyWhenSeen(t *testing.T) {
	p := removalIncident(n3Removed(""), unavailable("web"))

	msg := Writer{}.Write(announce(p), writerNow)

	assert.Contains(t, msg.Note, "web in shop has no ready replicas "+
		"(0 of 1) because node n3 was removed at 09:50 UTC.")
	assert.NotContains(t, msg.Note, "room")
}

// With no failing workload the removed node is the subject: it "was
// removed", it is not "down".
func TestRemovalLeadSaysANodeWasRemovedNotDown(t *testing.T) {
	gone := detection.Finding{
		Entity: inventory.CoreID(kube.KindService, "shop", "web"),
		Reason: reasons.ServiceNoEndpoints, Severity: detection.Critical,
		Since: writerNow.Add(-5 * time.Minute), Symptom: true,
		Summary: "Service has no ready backends; traffic to it fails"}
	p := removalIncident(n3Removed(""), gone)

	msg := Writer{}.Write(announce(p), writerNow)

	assert.Contains(t, msg.Note, "Node n3 was removed at 09:50 UTC.")
	assert.NotContains(t, msg.Note, "is down")
}

// A node that is still there was not removed: a change to it keeps
// "is down".
func TestChangedNodeIsStillDown(t *testing.T) {
	change := inventory.Change{Entity: removedN3,
		At: writerNow.Add(-10 * time.Minute),
		Fields: []inventory.FieldChange{{Path: "metadata.labels.pool",
			After: "gpu"}}}
	pending := detection.Finding{
		Entity: inventory.CoreID(kube.KindService, "shop", "web"),
		Reason: reasons.ServiceNoEndpoints, Severity: detection.Critical,
		Since: writerNow.Add(-5 * time.Minute), Symptom: true,
		Summary: "Service has no ready backends; traffic to it fails"}
	p := removalIncident(change, pending)

	msg := Writer{}.Write(announce(p), writerNow)

	assert.Contains(t, msg.Note, "Node n3 is down after the 09:50 UTC "+
		"change")
	assert.NotContains(t, msg.Note, "removed")
}

// removalUpdate is the update after api and web began failing in an
// incident whose blamed change is the given one.
func removalUpdate(change inventory.Change) string {
	api, web := unavailable("api"), unavailable("web")
	p := removalIncident(change, api, web)
	p.Revision, p.ReportedKnown = 2, true
	for _, s := range []detection.Finding{api, web} {
		p.Timeline = append(p.Timeline, incident.Event{At: writerNow,
			Text: s.Summary, Entity: aboutEntity(s.Entity)})
	}
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}
	return Writer{}.Write(d, writerNow).Note
}

// The update after a removal leads with the workloads, as the first
// message does, and says the removal is why.
func TestRemovalUpdateLeadsWithTheWorkloads(t *testing.T) {
	note := removalUpdate(n3Removed(""))

	assert.Contains(t, note, "api and web in shop now have no ready "+
		"replicas (0 of 1) because node n3 was removed at 09:50 UTC.")
	assert.NotContains(t, note, "now also affecting")
}

// A node that was not removed stays the subject of the update.
func TestChangedNodeUpdateLeadsWithTheNode(t *testing.T) {
	change := n3Removed("")
	change.Deleted = false

	note := removalUpdate(change)

	assert.Contains(t, note, "Node n3 is now also affecting api and web")
	assert.NotContains(t, note, "removed")
}
