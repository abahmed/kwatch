package explain

import "testing"

func TestScoreSharedRewardsSeveralWorkloads(t *testing.T) {
	for _, tc := range []struct {
		name      string
		workloads []string
		want      float64
	}{
		{"three workloads meet on the node",
			[]string{"api", "web", "cart"}, SharedWeight},
		{"one workload on a shared node", []string{"api"},
			-SharedPenalty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			node, pods := nodeWithPods(f, 1, tc.workloads...)
			f.fail(node, "NotReady", failingH, 1, "")
			for _, pod := range pods {
				f.fail(pod, "NotReady", failingH, 2, "")
			}
			requireWeight(t, scoreOf(t, f, node, scoreShared), tc.want)
		})
	}
}
