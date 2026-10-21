package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestScoreCoverageComparesFailingDependents(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failing int
		want    float64
	}{
		{"every pod fails", 4, CoverageWeight},
		{"one pod in four fails", 1, -CoverageWeight / 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			node, pods := nodeWithPods(f, 4, "api")
			f.fail(node, "NotReady", failingH, 1, "")
			for _, pod := range pods[:tc.failing] {
				f.fail(pod, "NotReady", failingH, 2, "")
			}
			requireWeight(t, scoreOf(t, f, node, scoreCoverage), tc.want)
		})
	}
}

// A policy depends on nothing; the pods it selects are its dependents.
func TestScoreCoverageCountsThePodsAPolicySelects(t *testing.T) {
	f, policy := classCase(t, kube.KindNetworkPolicy, "",
		"dial tcp 10.0.3.4:5432: i/o timeout")

	requireWeight(t, scoreOf(t, f, policy, scoreCoverage), CoverageWeight)
}

// The proof counts every dependent of the cause, not a sample of them:
// a node with fifty pods says "49 of 50", never "31 of 32".
func TestScoreCoverageCountsEveryDependent(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 50, "api")
	f.fail(node, "NotReady", failingH, 1, "")
	for _, pod := range pods[:49] {
		f.fail(pod, "NotReady", failingH, 2, "")
	}
	o := scoreOf(t, f, node, scoreCoverage)
	if o.count != 49 || o.total != 50 {
		t.Fatalf("count = %d of %d, want 49 of 50", o.count, o.total)
	}
}
