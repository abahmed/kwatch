package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// crashedRuns gives the first failing of the n pods of web a container
// that exited with an error and the given termination message.
func crashedRuns(
	f *fixture, n, failing int, message string,
) inventory.EntityID {
	pods := f.workload("shop", "web", n)
	for _, pod := range pods[:failing] {
		f.fail(containerOf(pod), detection.Mode(reasons.Error), failingH,
			2, message)
	}
	return inventory.CoreID(kube.KindDeployment, "shop", "web")
}

// TestSelfQuotedRewardsEveryRunQuotingItsOwnError: every replica of a
// workload blamed for itself fails with the program's own words.
func TestSelfQuotedRewardsEveryRunQuotingItsOwnError(t *testing.T) {
	f := newFixture(t)
	web := crashedRuns(f, 3, 3, "panic: table is locked")

	requireWeight(t, scoreOf(t, f, web, scoreSelfQuoted), SelfQuotedWeight)
	c := requireCause(t, f.explain(), containerOf(
		inventory.CoreID(kube.KindPod, "shop", "web-1-0")), web.String())
	found := false
	for _, contribution := range c.Contributions {
		found = found || contribution.Code == rootcause.ProofOwnErrors
	}
	if !found {
		t.Fatalf("no own-errors proof in %+v", c.Contributions)
	}
}

// TestSelfQuotedNeedsEveryRunToFail: a healthy replica leaves the
// program's words without the pattern that makes them its own fault.
func TestSelfQuotedNeedsEveryRunToFail(t *testing.T) {
	f := newFixture(t)
	web := crashedRuns(f, 3, 2, "panic: table is locked")

	if o := scoreOf(t, f, web, scoreSelfQuoted); o.said() {
		t.Fatalf("partial failure was rewarded: %+v", o)
	}
}

// TestSelfQuotedNeedsTheProgramsOwnWords: a message the kubelet writes
// (an empty termination message, a back-off) is no quoted error.
func TestSelfQuotedNeedsTheProgramsOwnWords(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "web", 2)
	for _, pod := range pods {
		f.fail(containerOf(pod), detection.Mode(reasons.CrashLoopBackOff),
			failingH, 2, "back-off restarting failed container")
	}
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")

	if o := scoreOf(t, f, web, scoreSelfQuoted); o.said() {
		t.Fatalf("kubelet words were taken as the program's: %+v", o)
	}
}

// TestSelfQuotedSkipsADependencysError: every replica quotes a refused
// connection to its database. Those words are about the database, not
// the program, so they earn the workload no reward.
func TestSelfQuotedSkipsADependencysError(t *testing.T) {
	f := newFixture(t)
	web := crashedRuns(f, 3, 3,
		"dial tcp 10.0.0.7:5432: connect: connection refused")
	requireWeight(t, scoreOf(t, f, web, scoreSelfQuoted), 0)
}
