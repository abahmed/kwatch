package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func stepsFor(root inventory.EntityID, members ...detection.Finding) string {
	p := incident.Incident{Root: root}
	var out []string
	for _, step := range nextSteps(p, members) {
		out = append(out, step.Command)
	}
	return strings.Join(out, "\n")
}

func TestStepsSkipPreviousLogsForContainersThatNeverRan(t *testing.T) {
	id := inventory.CoreID(kube.KindContainer, "ns", "p/app")
	for _, r := range []string{
		reasons.ImagePullBackOff, reasons.ErrImagePull,
		reasons.CreateConfigError,
	} {
		got := stepsFor(id, detection.Finding{Entity: id, Reason: r})
		if strings.Contains(got, "--previous") ||
			!strings.Contains(got, "kubectl describe pod p -n ns") {
			t.Errorf("%s: steps = %q", r, got)
		}
	}
}

func TestStepsNeverPrintSecretValues(t *testing.T) {
	got := stepsFor(inventory.CoreID(kube.KindSecret, "ns", "db"))
	if strings.Contains(got, "-o yaml") || strings.Contains(got, "-o json ") {
		t.Fatalf("secret values exposed: %q", got)
	}
	want := "kubectl describe secret db -n ns"
	if got != want {
		t.Errorf("steps = %q want %q", got, want)
	}
}

func TestStepsSkipDescribeForVirtualKinds(t *testing.T) {
	for _, id := range []inventory.EntityID{
		inventory.CoreID(kube.KindRegistry, "", "reg.io"),
		inventory.CoreID(kube.KindImage, "", "reg.io/a:1"),
		inventory.CoreID(kube.KindZone, "", "z1"),
		kube.ClusterDNS,
		inventory.CoreID(rootcause.KindScheduling, "", "Insufficient cpu"),
	} {
		if got := stepsFor(id, detection.Finding{Entity: id}); got != "" {
			t.Errorf("%s: steps = %q", id, got)
		}
	}
}

func TestStepsShellQuoteNames(t *testing.T) {
	id := inventory.CoreID(kube.KindService, "ns", "a b;rm -rf")
	got := stepsFor(id, detection.Finding{Entity: id})
	if got != "kubectl describe service 'a b;rm -rf' -n ns" {
		t.Errorf("steps = %q", got)
	}
	if q := quote("it's"); q != `'it'\''s'` {
		t.Errorf("quote = %s", q)
	}
}

func TestRolloutUndoNeedsKnownRevision(t *testing.T) {
	dep := inventory.CoreID(kube.KindDeployment, "ns", "api")
	cause := &rootcause.CauseRecord{Change: &inventory.Change{Entity: dep}}
	p := incident.Incident{Root: dep, Cause: cause}

	for _, step := range nextSteps(p, nil) {
		if strings.Contains(step.Command, "undo") {
			t.Errorf("undo without a revision: %q", step.Command)
		}
	}
	cause.RollbackRevision = "4"
	steps := nextSteps(p, nil)
	last := steps[len(steps)-1]
	if last.Command != "kubectl rollout undo deployment/api -n ns "+
		"--to-revision=4" || !last.Mutating {
		t.Errorf("undo step = %+v", last)
	}
}
