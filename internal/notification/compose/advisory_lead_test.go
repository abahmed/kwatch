package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var advisoryNow = time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)

func riskFinding(root inventory.EntityID) detection.Finding {
	return detection.Finding{Entity: root,
		Reason:   reasons.RiskSingleReplica,
		Severity: detection.Info, Advisory: true, Since: advisoryNow,
		Summary: "Runs a single replica, so any restart is downtime"}
}

func advisoryIncident(
	root inventory.EntityID, members ...detection.Finding,
) incident.Incident {
	p := incident.Incident{ID: "inc-1", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: advisoryNow, Revision: 2,
		Members: map[detection.Key]detection.Finding{}}
	for _, m := range members {
		p.Members[m.Key()] = m
	}
	return p
}

// A risk is never the state of a message, even when it is the only
// member left after the failure recovered.
func TestAdvisoryNeverLeadsAnUpdate(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "staging", "shield")
	p := advisoryIncident(root, riskFinding(root))
	p.Timeline = []incident.Event{
		{At: advisoryNow, Text: incident.RecoveredPrefix +
			"Pod is crash looping (pod staging/shield-1)"},
	}
	for _, reason := range []incident.Reason{"material change", ""} {
		d := incident.Decision{Action: incident.Update, Incident: p,
			Reason: reason}
		msg := Writer{}.Write(d, advisoryNow)
		for _, bad := range []string{"single replica", "now failing",
			"is failing"} {
			if strings.Contains(msg.Title+msg.Note, bad) {
				t.Fatalf("%q leads with the risk: %s", reason, msg.Note)
			}
		}
	}
}

// A risk that joins an incident is not "spreading", and a failure that
// is still open keeps leading.
func TestAdvisoryIsNotSpreadAndFailureLeads(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "staging", "shield")
	pod := inventory.CoreID(kube.KindPod, "staging", "shield-1")
	crash := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff",
		Severity: detection.Warning, Since: advisoryNow,
		Summary: "Pod keeps crashing and is restarting with back-off"}
	risk := riskFinding(root)
	p := advisoryIncident(root, crash, risk)
	p.Timeline = []incident.Event{
		{At: advisoryNow, Text: risk.Summary +
			" (deployment staging/shield)"}}
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: "material change"}
	note := Writer{}.Write(d, advisoryNow).Note
	if strings.Contains(note, "now failing") ||
		strings.Contains(note, "spreading") {
		t.Fatalf("a risk must not be reported as new failure: %s", note)
	}
	f := gatherFacts(incident.Decision{Incident: p}, advisoryNow, nil)
	if f.lead.Advisory {
		t.Fatal("the lead finding must not be advisory")
	}
}

// Self cause: no "because it is failing on its own" in an announcement,
// an update that revises the cause, or a roll-up title.
func TestSelfCauseNeverSaysBecauseAtAnyCertainty(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "staging", "transactions")
	svc := inventory.CoreID(kube.KindService, "staging", "transactions")
	backends := detection.Finding{Entity: svc, Reason: "NoEndpoints",
		Severity: detection.Critical, Symptom: true, Since: advisoryNow,
		Summary: "Service has no ready backends; traffic to it fails"}
	for _, score := range []float64{rootcause.High, rootcause.Likely, 0.1} {
		p := advisoryIncident(root, backends)
		p.Cause = &rootcause.CauseRecord{Root: root, Rule: "self",
			Score: score, Chain: []inventory.EntityID{root}}
		for _, action := range []incident.Action{incident.Announce,
			incident.Update} {
			d := incident.Decision{Action: action, Incident: p,
				Reason: incident.ReasonCauseRevised}
			msg := Writer{}.Write(d, advisoryNow)
			text := msg.Title + " " + msg.Note
			for _, bad := range []string{"because", "on its own"} {
				if strings.Contains(text, bad) {
					t.Fatalf("score %v %v: %q in %s", score, action,
						bad, text)
				}
			}
		}
	}
}
