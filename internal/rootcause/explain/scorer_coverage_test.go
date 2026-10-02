package explain

import "testing"

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
