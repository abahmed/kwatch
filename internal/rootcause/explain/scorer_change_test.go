package explain

import "testing"

func TestScoreChangeRewardsRecentChanges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
		want  float64
	}{
		{"taint added before the failures", []string{"spec.taints"},
			ChangeWeight},
		{"replica count only", []string{"spec.replicas"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			node, pods := nodeWithPods(f, 2, "api")
			f.fail(node, "NotReady", failingH, 1, "")
			f.change(node, 1, tc.paths...)
			for _, pod := range pods {
				f.fail(pod, "NotReady", failingH, 2, "")
			}
			requireWeight(t, scoreOf(t, f, node, scoreChange), tc.want)
		})
	}
}
