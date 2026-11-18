package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// helmRowCases cover the Helm release rows.
var helmRowCases = []rowCase{
	{row: RuleHelmReleaseFailed, build: func(f *fixture) inventory.EntityID {
		pods := helmCase(f, detection.ModeHelmReleaseFailed, 2, 3)
		return containerOf(pods[0])
	}, want: "helmrelease/shop/payments"},
}

// helmCase deploys release payments in shop with Deployment payments,
// whose revision began minutes after t0 and is in mode; both pods of
// the Deployment crash from failMinutes after t0.
func helmCase(
	f *fixture, mode detection.Mode, minutes, failMinutes int,
) []inventory.EntityID {
	release := kube.HelmReleaseID("shop", "payments")
	began := t0.Add(time.Duration(minutes) * time.Minute)
	f.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: release, Attributes: map[string]inventory.Value{
			kube.AttrHelmRevisionCreated: inventory.Time(began),
			kube.AttrHelmStatusSince:     inventory.Time(began)}})
	f.fail(release, mode, failingH, minutes, "")
	pods := f.workload("shop", "payments", 2)
	f.relate(inventory.CoreID(kube.KindDeployment, "shop", "payments"),
		inventory.DeployedBy, release)
	for _, pod := range pods {
		f.fail(containerOf(pod), detection.ModeCrashLoop, failingH,
			failMinutes, "panic: missing key DB_URL")
	}
	return pods
}

// TestHelmReleaseIsPossibleUnlessTheChangeNamesIt checks the certainty
// of the release: possible from the marks alone, likely or better when
// the workload's change was made by the release.
func TestHelmReleaseIsPossibleUnlessTheChangeNamesIt(t *testing.T) {
	f := newFixture(t)
	pods := helmCase(f, detection.ModeHelmReleaseFailed, 2, 3)
	c := requireCause(t, f.explain(), containerOf(pods[0]),
		"helmrelease/shop/payments")
	if c.Confidence >= ConfidenceLikely {
		t.Fatalf("unclaimed release scored %.2f, want possible",
			c.Confidence)
	}

	f = newFixture(t)
	pods = helmCase(f, detection.ModeHelmReleaseFailed, 2, 3)
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "payments")
	f.changes[deployment] = append(f.changes[deployment], inventory.Change{
		Entity: deployment, At: t0.Add(2 * time.Minute),
		App: "helm/payments", Fields: []inventory.FieldChange{
			{Path: "spec.template.spec.containers[app].image",
				After: "x"}}})
	c = requireCause(t, f.explain(), containerOf(pods[0]),
		"helmrelease/shop/payments")
	if rootcause.LevelOf(c.Confidence) == rootcause.LevelPossible {
		t.Fatalf("claimed release scored %.2f, want at least likely",
			c.Confidence)
	}
}

// TestHelmReleaseDoesNotExplainEarlierFailures checks a workload that
// was failing before the revision began is not blamed on the release.
func TestHelmReleaseDoesNotExplainEarlierFailures(t *testing.T) {
	f := newFixture(t)
	pods := helmCase(f, detection.ModeHelmReleaseFailed, 6, 1)
	e := f.explain()
	if c, ok := e.CauseOf(containerOf(pods[0])); ok &&
		c.Root.Kind == kube.KindHelmRelease {
		t.Fatalf("release blamed for a failure before it: %+v", c)
	}
}

// TestHealthyHelmReleaseIsNotWalked checks a release in no failing mode
// is neither blamed nor listed as checked.
func TestHealthyHelmReleaseIsNotWalked(t *testing.T) {
	f := newFixture(t)
	pods := helmCase(f, detection.ModeHelmReleaseFailed, 2, 3)
	release := kube.HelmReleaseID("shop", "payments")
	delete(f.findings, release)
	e := f.explain()
	effect := containerOf(pods[0])
	if c, ok := e.CauseOf(effect); ok && c.Root == release {
		t.Fatalf("healthy release blamed: %+v", c)
	}
	for _, area := range e.Areas {
		for _, checked := range area.Trace.Checked[effect] {
			if checked == release {
				t.Fatal("a healthy release is listed as checked")
			}
		}
	}
}

// TestHelmReleaseOfAnotherWorkloadIsNotBlamed checks a failing
// workload whose marks name another release is not linked.
func TestHelmReleaseOfAnotherWorkloadIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	helmCase(f, detection.ModeHelmReleaseFailed, 2, 3)
	other := f.workload("shop", "orders", 1)
	f.fail(containerOf(other[0]), detection.ModeCrashLoop, failingH, 4,
		"panic: boom")
	e := f.explain()
	if c, ok := e.CauseOf(containerOf(other[0])); ok &&
		c.Root.Kind == kube.KindHelmRelease {
		t.Fatalf("release of another workload blamed: %+v", c)
	}
}

// TestHelmReleaseDoesNotExplainMuchLaterFailures checks a workload
// that began failing long after the revision did is not blamed on it.
func TestHelmReleaseDoesNotExplainMuchLaterFailures(t *testing.T) {
	f := newFixture(t)
	f.now = t0.Add(3 * time.Hour)
	pods := helmCase(f, detection.ModeHelmReleaseFailed, 2, 150)
	e := f.explain()
	if c, ok := e.CauseOf(containerOf(pods[0])); ok &&
		c.Root.Kind == kube.KindHelmRelease {
		t.Fatalf("release blamed for a much later failure: %+v", c)
	}
}
