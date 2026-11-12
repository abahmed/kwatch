package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// The kubelet's own pull rate limit is the node's, never a registry's
// refusal, even when the back-off message hides it.
func TestClassifyPullQPSExceededIsTheNode(t *testing.T) {
	for _, text := range []string{
		"pull QPS exceeded",
		`Back-off pulling image "r.example/app:1" Failed to pull ` +
			`image "r.example/app:1": pull QPS exceeded.`,
	} {
		if got := classifyRegistryPull(text); got != pullNodeThrottled {
			t.Errorf("classifyRegistryPull(%q) = %q, want %q", text, got,
				pullNodeThrottled)
		}
	}
}

// pullThrottleRowCases: pods of two workloads on n1 cannot pull because
// n1's kubelet holds the pulls back.
var pullThrottleRowCases = []rowCase{
	{row: RowNodePullsThrottled, want: "node//n1",
		build: func(f *fixture) inventory.EntityID {
			nodes := f.nodes("zone-a", "n1")
			var first inventory.EntityID
			for _, name := range []string{"api", "worker"} {
				pod := f.workload("shop", name, 1, nodes...)[0]
				f.fail(containerOf(pod), detection.ModeImagePull, failingH,
					2, "pull QPS exceeded")
				if first.Name == "" {
					first = containerOf(pod)
				}
			}
			return first
		}},
}
