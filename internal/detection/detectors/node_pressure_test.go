package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func pressuredNode(condition string) (*inventory.Model, inventory.EntityID) {
	m := inventory.NewModel(inventory.Options{})
	node := inventory.EntityID{Kind: kube.KindNode, Name: "worker-9"}
	put(m, node, t0, conditionAttrs(nil, condition, "True", "Kubelet", t0))
	return m, node
}

func TestNodePressureWaitsForTheSustainWindow(t *testing.T) {
	for _, tc := range []struct{ condition, reason string }{
		{"MemoryPressure", reasons.MemoryPressure},
		{"DiskPressure", reasons.DiskPressure},
		{"PIDPressure", reasons.PIDPressure},
	} {
		t.Run(tc.condition, func(t *testing.T) {
			m, node := pressuredNode(tc.condition)
			r := detection.NewRegistry(nil, NewNode(0))

			early := r.Evaluate(m, t0.Add(30*time.Second), node)
			assert.Empty(t, early.Findings, "a brief spike is not news")
			assert.Equal(t, DefaultNodePressure-30*time.Second,
				early.RecheckAfter, "re-check, never poll")

			late := r.Evaluate(m, t0.Add(DefaultNodePressure), node)
			require.Len(t, late.Findings, 1)
			assert.Equal(t, tc.reason, late.Findings[0].Reason)
			assert.Equal(t, detection.Critical, late.Findings[0].Severity)
		})
	}
}
