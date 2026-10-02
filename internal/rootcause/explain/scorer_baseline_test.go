package explain

import "testing"

func TestScoreBaselineFollowsDeviation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		baseline fakeBaseline
		want     float64
		said     bool
	}{
		{"no baseline reader", nil, 0, false},
		{"far from its baseline", fakeBaseline{}, BaselineWeight, true},
		{"as usual", fakeBaseline{}, -BaselineWeight, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			node, pods := nodeWithPods(f, 2, "api")
			f.fail(node, "NotReady", failingH, 1, "")
			for _, pod := range pods {
				f.fail(pod, "NotReady", failingH, 2, "")
			}
			s := f.snapshot()
			s.Baseline = nil
			if tc.baseline != nil {
				tc.baseline[node] = (tc.want/BaselineWeight + 1) / 2
				s.Baseline = tc.baseline
			}
			o := scoreSnapshot(t, s, node, scoreBaseline)
			if o.said() != tc.said {
				t.Fatalf("said = %v, want %v", o.said(), tc.said)
			}
			requireWeight(t, o, tc.want)
		})
	}
}
