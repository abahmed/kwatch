package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

const lookAtApp = "so look at the app itself"

// checkedText is the note of an incident without a cause whose
// healthy node was checked.
func checkedText(p incident.Incident) string {
	p.CauseUnclear = false
	p.Checked = []string{"node"}
	return notification.Text(Writer{}.Write(announce(p), at(6, 0)))
}

// A crash of a workload with nothing else to look at points at the app.
func TestCheckedSentenceLooksAtTheAppOfACrash(t *testing.T) {
	text := checkedText(unclearIncident(false))
	if !strings.Contains(text, "Cause not found yet: its node is "+
		"healthy and unchanged, "+lookAtApp+".") {
		t.Fatalf("note = %s", text)
	}
}

// The tail is only for an app's own failure: not with a blind spot, a
// shared factor Pro found, a failure that is not a crash, or a root that
// is not a workload.
func TestCheckedSentenceTailOnlyForAppFailures(t *testing.T) {
	unverified := unclearIncident(false)
	unverified.Unverified = []string{"secret"}
	shared := unclearIncident(false)
	shared.ProCause = true
	oom := unclearIncident(false)
	for k, m := range oom.Members {
		m.Reason, m.Mode = reasons.OOMKilled, detection.ModeOOMKilled
		oom.Members[k] = m
	}
	job := unclearIncident(false)
	job.Root = inventory.CoreID(kube.KindJob, "shop", "reports")
	for name, p := range map[string]incident.Incident{
		"unverified": unverified, "pro cause": shared, "oom": oom,
		"job": job,
	} {
		text := checkedText(p)
		if strings.Contains(text, lookAtApp) ||
			!strings.Contains(text, "healthy and unchanged.") {
			t.Errorf("%s: note = %s", name, text)
		}
	}
}

// A node alert says nothing about what was checked around it.
func TestCheckedSentenceSilentForANode(t *testing.T) {
	n1 := inventory.CoreID(kube.KindNode, "", "n1")
	p := incident.Incident{ID: "inc-7", Root: n1, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(detection.Finding{Entity: n1,
			Reason: "NodeNotReady", Severity: detection.Critical,
			Since: at(0, 30), Summary: "Node is not ready",
			Symptom: true})}
	if text := checkedText(p); strings.Contains(text, "Cause not found") {
		t.Fatalf("note = %s", text)
	}
}

// A setting to fix names the place already: no "Cause not found yet".
func TestCheckedSentenceSilentWithAFixPlace(t *testing.T) {
	p := unclearIncident(false)
	for k, m := range p.Members {
		m.Place = &detection.Place{Container: "app",
			Path: "resources.limits.memory", Value: "64Mi"}
		p.Members[k] = m
	}
	text := checkedText(p)
	if strings.Contains(text, "Cause not found") ||
		!strings.Contains(text, "Fix: ") {
		t.Fatalf("note = %s", text)
	}
}

// A Community copy drops the recent change, but the change was seen:
// it does not send the reader to the app either.
func TestCommunityCheckedSentenceKeepsTheChangeSeen(t *testing.T) {
	d := announce(unclearIncident(false))
	d.Incident.CauseUnclear = false
	d.Incident.Checked = []string{"node"}
	d.Facts.Changes = []inventory.Change{{Entity: workload("reports"),
		At: at(0, 0), Actor: "alice", Fields: []inventory.FieldChange{{
			Path: "image", Before: "v1", After: "v2"}}}}
	community := Writer{}.ForEdition(feature.CommunityEntitlements(), true)
	text := notification.Text(community.Write(d, at(6, 0)))
	if strings.Contains(text, lookAtApp) ||
		!strings.Contains(text, "healthy and unchanged.") {
		t.Fatalf("note = %s", text)
	}
}
