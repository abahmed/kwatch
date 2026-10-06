package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func spreadFinding(name string, minute int) detection.Finding {
	return detection.Finding{
		Entity:   inventory.CoreID(kube.KindService, "shop", name),
		Reason:   "NoEndpoints",
		Severity: detection.Critical, Since: at(minute, 0), Symptom: true,
		Summary: "Service has no ready backends; traffic to it fails",
	}
}

func spreadUpdate(sinceSent bool) string {
	p := badRollout()
	cart, search := spreadFinding("cart", 7), spreadFinding("search", 8)
	p.Members[cart.Key()] = cart
	p.Members[search.Key()] = search
	p.Revision = 2
	p.Timeline = []incident.Event{
		{At: at(2, 20), Text: "Container is crash looping " +
			"(container shop/payments-7d9f/app)"},
		{At: at(7, 0), Text: cart.Summary + " (service shop/cart)",
			Entity: aboutEntity(inventory.CoreID(kube.KindService, "shop", "cart"))},
		{At: at(8, 0), Text: search.Summary + " (service shop/search)",
			Entity: aboutEntity(inventory.CoreID(kube.KindService, "shop", "search"))},
	}
	if sinceSent {
		// The last message reported the crash loop, the first entry.
		p.Reported, p.ReportedKnown = 1, true
	}
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: "material change"}
	return Writer{}.Write(d, at(8, 0)).Note
}

func TestUpdateNamesEverythingSinceLastMessage(t *testing.T) {
	note := spreadUpdate(true)
	for _, name := range []string{"cart", "search"} {
		if !strings.Contains(note, name) {
			t.Fatalf("update must name %s: %s", name, note)
		}
	}
	if !strings.Contains(note,
		"payments in shop is spreading to services") {
		t.Fatalf("affected objects stay grouped: %s", note)
	}
}

func TestUpdateWithoutSentMarkNamesLatestBatchOnly(t *testing.T) {
	note := spreadUpdate(false)
	if strings.Contains(note, "cart") || !strings.Contains(note, "search") {
		t.Fatalf("fallback names the latest batch: %s", note)
	}
}

func TestUpdateRecoveredMemberNamesTheSubject(t *testing.T) {
	p := badRollout()
	p.Revision = 2
	p.Timeline = []incident.Event{
		{At: at(2, 20), Text: "Container is crash looping " +
			"(container shop/payments-7d9f/app)"},
		{At: at(9, 0), Text: incident.RecoveredPrefix +
			"Service has no ready backends (service shop/cart)",
			Entity: aboutEntity(inventory.CoreID(
				kube.KindService, "shop", "cart"))},
	}
	p.Reported, p.ReportedKnown = 1, true
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: "material change"}

	short := Writer{}.Write(d, at(9, 0)).Short

	want := "🔴 Service cart has recovered; payments in shop still has " +
		"one open failure."
	if short != want {
		t.Fatalf("short = %q, want %q", short, want)
	}
}

func TestCauseRevisedAfterChangeLeadsWithSubject(t *testing.T) {
	d := incident.Decision{Action: incident.Update, Incident: badRollout(),
		Reason: incident.ReasonCauseRevised}

	short := Writer{}.Write(d, at(9, 0)).Short

	want := "🔴 payments in shop has a revised cause: it is down after " +
		"the 14:02 release of payments:2.3."
	if short != want {
		t.Fatalf("short = %q, want %q", short, want)
	}
}

func TestReplicaCountsReadPlainly(t *testing.T) {
	id := inventory.CoreID(kube.KindDeployment, "shop", "cart")
	cases := map[string]string{
		"0 of 1 replicas are ready": "now has no ready replicas (0 of 1)",
		"1 of 3 replicas are ready": "now has 1 of 3 replicas ready",
	}
	for summary, want := range cases {
		if got := nowState(predicate(id, summary)); got != want {
			t.Errorf("nowState(%q) = %q, want %q", summary, got, want)
		}
	}
}

// TestUpdateNamesTheSubjectOnce: when the subject's own condition
// leads the update, the spreading sentence that follows says "It"
// instead of naming the subject a second time.
func TestUpdateNamesTheSubjectOnce(t *testing.T) {
	p := badRollout()
	down := detection.Finding{
		Entity: payments, Reason: "DeploymentUnavailable",
		Severity: detection.Critical, Since: at(7, 0),
		Summary: "0 of 1 replicas are ready",
	}
	cart := spreadFinding("cart", 7)
	p.Members[down.Key()] = down
	p.Members[cart.Key()] = cart
	p.Revision = 2
	p.Timeline = []incident.Event{
		{At: at(2, 20), Text: "Container is crash looping " +
			"(container shop/payments-7d9f/app)"},
		{At: at(7, 0), Text: down.Summary + " (deployment shop/payments)",
			Entity: aboutEntity(payments)},
		{At: at(7, 0), Text: cart.Summary + " (service shop/cart)",
			Entity: aboutEntity(inventory.CoreID(kube.KindService, "shop", "cart"))},
	}
	p.Reported, p.ReportedKnown = 1, true
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: "material change"}

	note := Writer{}.Write(d, at(8, 0)).Note

	if strings.Count(note, "payments in shop") != 1 ||
		!strings.Contains(note, "It is spreading to service cart") {
		t.Fatalf("the subject must be named once: %s", note)
	}
}
