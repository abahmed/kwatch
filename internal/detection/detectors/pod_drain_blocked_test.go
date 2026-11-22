package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestUnownedPodBlocksDrain: a pod with no controller on a cordoned node
// is reported once the drain has waited DefaultBudgetBlocked; a pod a
// controller owns, or one on a schedulable node, is not.
func TestUnownedPodBlocksDrain(t *testing.T) {
	cases := map[string]struct {
		cordoned, owned, want bool
	}{
		"bare pod on a cordoned node": {true, false, true},
		"bare pod on a ready node":    {false, false, false},
		"owned pod, cordoned node":    {true, true, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			pod := newID(kube.KindPod, "ops", "shell")
			put(m, pod, t0, map[string]inventory.Value{
				kube.AttrPhase: inventory.Text("Running"),
				kube.AttrReady: inventory.Bool(true)})
			node := newID(kube.KindNode, "", "n1")
			put(m, node, t0, map[string]inventory.Value{
				kube.AttrUnschedulable: inventory.Bool(c.cordoned)})
			link(m, pod, inventory.RunsOn, node)
			if c.owned {
				rs := newID(kube.KindReplicaSet, "ops", "rs")
				put(m, rs, t0, nil)
				link(m, pod, inventory.OwnedBy, rs)
			}
			early := evaluate(Pod{}, m, t0.Add(5*time.Minute), pod, nil)
			assert.Empty(t, early.Findings)
			late := evaluate(Pod{}, m, t0.Add(11*time.Minute), pod, nil)
			if !c.want {
				assert.Empty(t, late.Findings)
				return
			}
			require.Len(t, late.Findings, 1)
			assert.Equal(t, "Pod.BlocksDrain",
				string(late.Findings[0].Mode))
		})
	}
}
