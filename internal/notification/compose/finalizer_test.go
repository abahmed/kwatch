package compose

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

var stuckSince = time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)

// heldFacts are n objects of ns held by the finalizers.
func heldFacts(
	cause *rootcause.CauseRecord, ns string, n int, finalizers string,
) caseFacts {
	var members []detection.Finding
	for i := 0; i < n; i++ {
		id := inventory.CoreID(kube.KindSecret, ns,
			fmt.Sprintf("old-%d", i))
		members = append(members, detection.Finding{Entity: id,
			Reason: "StuckDeleting", Mode: detection.ModeStuckDeleting,
			Since: stuckSince.Add(time.Duration(i) * time.Hour),
			Evidence: []detection.Evidence{
				{Label: "finalizers", Value: finalizers}}})
	}
	return caseFacts{p: incident.Incident{Root: cause.Root, Cause: cause},
		members: members, cluster: "prod-eu-1"}
}

func controllerCause(mode detection.Mode) *rootcause.CauseRecord {
	return &rootcause.CauseRecord{Rule: "finalizer-handler-stopped",
		Mode: mode, Root: inventory.CoreID(kube.KindDeployment, "shop",
			"widget-ctl")}
}

func unhandledCause(mode detection.Mode) *rootcause.CauseRecord {
	return &rootcause.CauseRecord{Rule: "finalizer-unhandled", Mode: mode,
		Root: inventory.CoreID(explain.KindFinalizer, "shop",
			"widgets.example.com/cleanup")}
}

func TestFinalizerLeadNamesTheScaledDownController(t *testing.T) {
	f := heldFacts(controllerCause(explain.ModeHandlerScaledDown), "shop",
		6, "widgets.example.com/cleanup")

	got := leadSentences(f)

	want := "Six objects in shop (prod-eu-1) have been stuck deleting " +
		"since Oct 3: their finalizer widgets.example.com/cleanup is " +
		"never removed; the controller that handles it, deployment " +
		"shop/widget-ctl, is scaled to 0."
	if len(got) != 1 || got[0].text != want {
		t.Fatalf("got %+v\nwant %q", got, want)
	}
}

func TestFinalizerLeadSaysAControllerWithNoReadyReplica(t *testing.T) {
	f := heldFacts(controllerCause(explain.ModeHandlerNotReady), "shop",
		1, "widgets.example.com/cleanup")

	got := leadSentences(f)[0].text

	want := "One object (secret old-0) in shop (prod-eu-1) " +
		"has been stuck deleting " +
		"since Oct 3: its finalizer widgets.example.com/cleanup is " +
		"never removed; the controller that handles it, deployment " +
		"shop/widget-ctl, has no ready replica."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestFinalizerLeadWithNoControllerFound(t *testing.T) {
	f := heldFacts(unhandledCause(explain.ModeFinalizerUnhandled), "shop",
		3, "widgets.example.com/cleanup")

	got := leadSentences(f)[0].text

	want := "Three objects (secret old-0, secret old-1 and secret old-2) " +
		"in shop (prod-eu-1) have been stuck deleting " +
		"since Oct 3: their finalizer widgets.example.com/cleanup is " +
		"never removed; no controller in the cluster is known to " +
		"handle it (API group widgets.example.com)."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// A controller that runs is not accused, and the message does not claim
// that none is known.
func TestFinalizerLeadWithARunningController(t *testing.T) {
	f := heldFacts(unhandledCause(explain.ModeFinalizerHandlerRuns), "shop",
		2, "widgets.example.com/cleanup")

	got := leadSentences(f)[0].text

	want := "Two objects (secret old-0 and secret old-1) in shop " +
		"(prod-eu-1) have been stuck deleting " +
		"since Oct 3: their finalizer widgets.example.com/cleanup is " +
		"never removed; a controller that appears to handle it is " +
		"running, so kwatch cannot say why it does not remove it."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestFinalizerLeadNamesSeveralFinalizers(t *testing.T) {
	f := heldFacts(controllerCause(explain.ModeHandlerScaledDown), "shop",
		2, "a.example.com/x, b.example.com/y")

	got := leadSentences(f)[0].text

	want := "Two objects (secret old-0 and secret old-1) in shop " +
		"(prod-eu-1) have been stuck deleting " +
		"since Oct 3: their finalizers a.example.com/x and " +
		"b.example.com/y are never removed; the controller that " +
		"handles them, deployment shop/widget-ctl, is scaled to 0."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// Workloads named like the controller are said to be only that: named
// alike, with nothing that ties them to the group.
func TestFinalizerLeadNamesLookAlikesAsUnconfirmed(t *testing.T) {
	cause := unhandledCause(explain.ModeFinalizerUnhandled)
	cause.Proof = []rootcause.Proof{{
		Code:   rootcause.ProofNamedLikeHandler,
		Fields: []string{"deployment arc/actions-runner-controller"}}}
	f := heldFacts(cause, "shop", 2, "widgets.example.com/cleanup")

	got := leadSentences(f)[0].text

	want := "Two objects (secret old-0 and secret old-1) in shop " +
		"(prod-eu-1) have been stuck deleting since Oct 3: their " +
		"finalizer widgets.example.com/cleanup is never removed; no " +
		"controller in the cluster is known to handle it (API group " +
		"widgets.example.com); deployment arc/actions-runner-controller " +
		"is named like it, but nothing confirms it serves group " +
		"widgets.example.com."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestFinalizerLeadNamesSeveralLookAlikes(t *testing.T) {
	cause := unhandledCause(explain.ModeFinalizerUnhandled)
	cause.Proof = []rootcause.Proof{{
		Code:   rootcause.ProofNamedLikeHandler,
		Fields: []string{"deployment arc/a", "deployment arc/b"}}}
	f := heldFacts(cause, "shop", 2, "widgets.example.com/cleanup")

	got := leadSentences(f)[0].text

	want := "(API group widgets.example.com); deployment arc/a and " +
		"deployment arc/b are named like it, but nothing confirms " +
		"they serve group widgets.example.com."
	if !strings.HasSuffix(got, want) {
		t.Fatalf("got %q\nwant suffix %q", got, want)
	}
}

// A finalizer that names no group (a CSI sidecar's) says no group.
func TestFinalizerLeadWithoutAGroupNamesNone(t *testing.T) {
	cause := unhandledCause(explain.ModeFinalizerUnhandled)
	f := heldFacts(cause, "", 2, "external-attacher/ebs-csi-example-com")

	got := leadSentences(f)[0].text

	if !strings.HasSuffix(got, "never removed; no controller in the "+
		"cluster is known to handle it.") || strings.Contains(got, "group") {
		t.Fatalf("got %q", got)
	}
}

// What the held objects say about themselves is quoted: a status error
// before an event, the object's own before a controller's.
func TestFinalizerLeadQuotesTheHeldObjectsStatusError(t *testing.T) {
	cause := unhandledCause(explain.ModeFinalizerUnhandled)
	f := heldFacts(cause, "", 2, "external-attacher/ebs-csi-example-com")
	for i := range f.members {
		f.members[i].Entity = inventory.NewEntityID("",
			kube.KindVolumeAttachment, "", fmt.Sprintf("csi-%d", i))
		f.members = append(f.members, detection.Finding{
			Entity: f.members[i].Entity, Reason: reasons.VolumeDetachFailure,
			Evidence: []detection.Evidence{{Label: "detach error",
				Value: "rpc error: PersistentVolume not found"}}})
	}

	got := leadSentences(f)[0].text

	want := "; the status of volume attachment csi-0 reports a detach " +
		"error: \"rpc error: PersistentVolume not found\"."
	if !strings.HasSuffix(plainQuotes(got), want) {
		t.Fatalf("got %q\nwant suffix %q", plainQuotes(got), want)
	}
}

// The reason of a deletion held by a controller kwatch cannot see is the
// Warning event it wrote; a long message is quoted from its end, where
// a wrapped error states its cause.
func TestFinalizerLeadQuotesTheLatestWarningEventFromItsEnd(t *testing.T) {
	cause := unhandledCause(explain.ModeFinalizerUnhandled)
	f := heldFacts(cause, "shop", 2, "service.k8s.aws/resources")
	long := "Failed deploy model due to operation error Elastic Load " +
		"Balancing v2: DeleteLoadBalancer, https response error " +
		"StatusCode: 400, RequestID: 1725b5e1, ResourceInUse: Load " +
		"balancer 'arn:aws:elasticloadbalancing:eu-west-1:1:loadbalancer" +
		"/net/k8s/863dfe' cannot be deleted because it is currently " +
		"associated with another service"
	f.members[0].Evidence = append(f.members[0].Evidence,
		detection.Evidence{Label: detection.EvidenceLastWarning,
			Value: long})

	got := plainQuotes(leadSentences(f)[0].text)

	if !strings.Contains(got, "the latest Warning event on secret old-0 "+
		"says: \"…") || !strings.HasSuffix(got, "cannot be deleted "+
		"because it is currently associated with another service\".") ||
		strings.Contains(got, "Failed deploy model") {
		t.Fatalf("got %q", got)
	}
}

// A single stuck object with no stated cause keeps the usual wording
// unless its controller wrote why it cannot finish.
func TestStuckObjectLeadQuotesTheLatestWarningEvent(t *testing.T) {
	id := inventory.CoreID(kube.KindService, "kube-addons", "ingress")
	lead := detection.Finding{Entity: id, Reason: reasons.StuckDeleting,
		Mode: detection.ModeStuckDeleting, Since: stuckSince,
		Summary: "Deletion is blocked by finalizers service.k8s.aws/resources",
		Evidence: []detection.Evidence{
			{Label: "finalizers", Value: "service.k8s.aws/resources"},
			{Label: detection.EvidenceLastWarning,
				Value: "ResourceInUse: cannot be deleted"}}}
	f := caseFacts{p: incident.Incident{Root: id},
		members: []detection.Finding{lead}, lead: lead, ok: true}

	got, ok := stuckObjectLead(f)

	if !ok || !strings.HasSuffix(plainQuotes(got), "hold it; the latest "+
		"Warning event on it says: \"ResourceInUse: cannot be deleted\"") {
		t.Fatalf("got %q (%v)", plainQuotes(got), ok)
	}
	lead.Evidence = lead.Evidence[:1]
	f.members, f.lead = []detection.Finding{lead}, lead
	if _, ok := stuckObjectLead(f); ok {
		t.Fatal("no event, so the usual wording stands")
	}
}
