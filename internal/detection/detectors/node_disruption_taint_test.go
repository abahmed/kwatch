package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Node autoscalers other than the cluster autoscaler, and the cloud
// provider for a node that shut down, announce a removal with their own
// documented taints: each reads as a drain too.
func TestNodeDisruptionTaintsOfOtherAutoscalersAreADrain(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	for taints, want := range map[string]string{
		"karpenter.sh/disrupted:NoSchedule":                    "disruption",
		"karpenter.sh/disruption=disrupting:NoSchedule":        "disruption",
		"node.cloudprovider.kubernetes.io/shutdown:NoSchedule": "shut down",
	} {
		node := buildNode("n1", now.Add(-time.Minute),
			map[string]inventory.Value{
				kube.AttrTaints:   inventory.Text(taints),
				"condition.Ready": inventory.Text("True"),
			})
		ok, drain := drainFinding(node)
		require.True(t, ok, taints)
		assert.Equal(t, reasons.NodeDraining, drain.Reason)
		assert.Contains(t, drain.Summary, want)
	}
}
