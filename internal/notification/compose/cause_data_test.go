package compose

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// TestWriterIgnoresExplainWording rewords everything explain says in
// prose, the cause summary and the text of each proof, and expects the
// same message: writers read the cause's mode, row and proof codes.
func TestWriterIgnoresExplainWording(t *testing.T) {
	cases := map[string]incident.Incident{
		"solved node outage": solvedNodeOutage(t),
		"endpoint storm": crashStorm(
			inventory.CoreID(explain.KindExternalEndpoint, "",
				"db.example.com:5432"),
			"external-endpoint-failing",
			explain.ModeEndpointFailing+".Refused", func(int) string {
				return "dial tcp db.example.com:5432: connect: " +
					"connection refused"
			}),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if len(p.Cause.Proof) == 0 {
				t.Fatal("the case needs proof to reword")
			}
			want := Writer{}.Write(announce(p), at(4, 0))
			got := Writer{}.Write(announce(reworded(p)), at(4, 0))
			if got.Note != want.Note || got.Short != want.Short {
				t.Fatalf("explain's wording reached the message\n"+
					" got: %s\nwant: %s", got.Note, want.Note)
			}
		})
	}
}

// reworded returns p with a cause whose prose explain worded otherwise.
func reworded(p incident.Incident) incident.Incident {
	cause := *p.Cause
	cause.Summary = "explain now words its summaries differently"
	cause.Proof = append([]rootcause.Proof(nil), p.Cause.Proof...)
	for i := range cause.Proof {
		cause.Proof[i].Text = "explain now words this differently"
	}
	p.Cause = &cause
	return p
}

// solvedNodeOutage solves a node that stopped reporting under two of
// three replicas of api, and returns the incident its cause opens.
func solvedNodeOutage(t *testing.T) incident.Incident {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	observe := func(id inventory.EntityID) {
		_, _ = model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", At: at(0, 0),
			Entity: id, Attributes: map[string]inventory.Value{},
		})
	}
	relate := func(from inventory.EntityID, rel inventory.RelationType,
		to inventory.EntityID) {
		_, _ = model.Apply(inventory.Observation{
			Kind: inventory.Related, Source: "test", At: at(0, 0),
			Entity: from, Relation: rel, Targets: []inventory.EntityID{to},
		})
	}
	n1 := inventory.CoreID(kube.KindNode, "", "n1")
	n2 := inventory.CoreID(kube.KindNode, "", "n2")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	observe(n1)
	observe(n2)
	observe(api)
	findings := map[inventory.EntityID][]detection.Finding{}
	fail := func(id inventory.EntityID, reason string, minute int) {
		findings[id] = []detection.Finding{detection.Classify(
			detection.Finding{Entity: id, Reason: reason,
				Severity: detection.Critical, Since: at(minute, 0),
				Summary: "Node is not ready"})}
	}
	fail(n1, reasons.NodeNotReady, 1)
	var failing []detection.Finding
	for i, on := range []inventory.EntityID{n1, n1, n2} {
		pod := inventory.CoreID(kube.KindPod, "shop",
			fmt.Sprintf("api-%d", i))
		observe(pod)
		relate(pod, inventory.RunsOn, on)
		relate(pod, inventory.OwnedBy, api)
		if on == n1 {
			fail(pod, reasons.PodStatusUnknown, 2)
			failing = append(failing, findings[pod]...)
		}
	}
	snapshot := explain.Snapshot{Model: model, Findings: findings,
		Now: at(3, 0)}
	cause, ok := explain.Explain(snapshot).CauseOf(failing[0].Entity)
	if !ok || cause.Root != n1 {
		t.Fatalf("cause = %+v, %v; want node n1", cause, ok)
	}
	record := snapshot.Record(cause)
	return incident.Incident{
		ID: "inc-1", Root: n1, Tier: incident.Notify, State: incident.Open,
		Opened: at(2, 0), Revision: 1,
		Members: members(append(failing, findings[n1]...)...),
		Impact:  []inventory.EntityID{api}, Cause: &record,
	}
}
