package explain

import "testing"

func TestScoreTemporalOrdersCauseBeforeEffect(t *testing.T) {
	for _, tc := range []struct {
		name        string
		nodeMinutes int
		want        float64
	}{
		{"cause first", 1, TemporalBefore},
		{"cause long after", 8, -TemporalAfter},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			node, pods := nodeWithPods(f, 2, "api")
			f.fail(node, "NotReady", failingH, tc.nodeMinutes, "")
			for _, pod := range pods {
				f.fail(pod, "NotReady", failingH, 3, "")
			}
			requireWeight(t, scoreOf(t, f, node, scoreTemporal), tc.want)
		})
	}
}
