package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
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

	want := "One object in shop (prod-eu-1) has been stuck deleting " +
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

	want := "Three objects in shop (prod-eu-1) have been stuck deleting " +
		"since Oct 3: their finalizer widgets.example.com/cleanup is " +
		"never removed; no running controller found for it."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// A controller that runs is not accused, and the message does not claim
// that none was found.
func TestFinalizerLeadWithARunningController(t *testing.T) {
	f := heldFacts(unhandledCause(explain.ModeFinalizerHandlerRuns), "shop",
		2, "widgets.example.com/cleanup")

	got := leadSentences(f)[0].text

	want := "Two objects in shop (prod-eu-1) have been stuck deleting " +
		"since Oct 3: their finalizer widgets.example.com/cleanup is " +
		"never removed; a controller that looks like its handler is " +
		"running, so kwatch cannot say why it does not remove it."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestFinalizerLeadNamesSeveralFinalizers(t *testing.T) {
	f := heldFacts(controllerCause(explain.ModeHandlerScaledDown), "shop",
		2, "a.example.com/x, b.example.com/y")

	got := leadSentences(f)[0].text

	want := "Two objects in shop (prod-eu-1) have been stuck deleting " +
		"since Oct 3: their finalizers a.example.com/x and " +
		"b.example.com/y are never removed; the controller that " +
		"handles them, deployment shop/widget-ctl, is scaled to 0."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
