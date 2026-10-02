package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestBudgetBlocksDrain: a budget that allows no eviction blocks the
// drain of the cordoned node its pod runs on, once the drain has waited
// DefaultBudgetBlocked; on a schedulable node it blocks nothing.
func TestBudgetBlocksDrain(t *testing.T) {
	for _, cordoned := range []bool{false, true} {
		m := newTestModel()
		pod := labelledPod(m, "ledger-1", "app=ledger")
		node := newID(kube.KindNode, "", "n2")
		put(m, node, t0, map[string]inventory.Value{
			kube.AttrUnschedulable: inventory.Bool(cordoned),
		})
		link(m, pod, inventory.RunsOn, node)
		id := budget(m, "ledger", "app=ledger", 1)
		put(m, id, t0, map[string]inventory.Value{
			kube.AttrSelector:           inventory.Text("app=ledger"),
			kube.AttrExpectedPods:       inventory.Number(1),
			kube.AttrDisruptionsAllowed: inventory.Number(0),
		})

		early := evaluate(Budget{}, m, t0.Add(5*time.Minute), id, nil)
		assert.Empty(t, early.Findings)
		late := evaluate(Budget{}, m, t0.Add(11*time.Minute), id, nil)
		if !cordoned {
			assert.Empty(t, late.Findings, "nothing is draining")
			continue
		}
		require.Len(t, late.Findings, 1)
		assert.Equal(t, "Budget.BlocksDrain", string(late.Findings[0].Mode))
		assert.Contains(t, late.Findings[0].Summary, "n2")
	}
}

// TestBudgetTooStrictBlocksDrainSoon: a budget that allows no eviction
// while every pod it expects is healthy can never let the drain go on,
// so it is reported after DefaultBudgetTooStrict, not after the full
// DefaultBudgetBlocked a progressing drain gets.
func TestBudgetTooStrictBlocksDrainSoon(t *testing.T) {
	cases := map[string]struct {
		healthy float64
		want    time.Duration
	}{
		"every pod healthy":      {1, DefaultBudgetTooStrict},
		"a replacement starting": {0, DefaultBudgetBlocked},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			pod := labelledPod(m, "ledger-1", "app=ledger")
			node := newID(kube.KindNode, "", "n2")
			put(m, node, t0, map[string]inventory.Value{
				kube.AttrUnschedulable: inventory.Bool(true),
			})
			link(m, pod, inventory.RunsOn, node)
			id := budget(m, "ledger", "app=ledger", 1)
			put(m, id, t0, map[string]inventory.Value{
				kube.AttrSelector:           inventory.Text("app=ledger"),
				kube.AttrExpectedPods:       inventory.Number(1),
				kube.AttrCurrentHealthy:     inventory.Number(c.healthy),
				kube.AttrDisruptionsAllowed: inventory.Number(0),
			})

			before := evaluate(Budget{}, m,
				t0.Add(c.want-time.Second), id, nil)
			after := evaluate(Budget{}, m, t0.Add(c.want), id, nil)

			assert.Empty(t, before.Findings)
			require.Len(t, after.Findings, 1)
			assert.Equal(t, "Budget.BlocksDrain", string(after.Findings[0].Mode))
		})
	}
}

// TestBudgetDrainIgnoresFinishedAndTerminatingPods: pods that are done
// or already going away do not hold up a drain.
func TestBudgetDrainIgnoresFinishedAndTerminatingPods(t *testing.T) {
	cases := map[string]map[string]inventory.Value{
		"succeeded": {kube.AttrPhase: inventory.Text("Succeeded")},
		"failed":    {kube.AttrPhase: inventory.Text("Failed")},
		"deleting":  {kube.AttrDeleting: inventory.Bool(true)},
	}
	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			pod := labelledPod(m, "ledger-1", "app=ledger")
			attrs[kube.AttrLabels] = inventory.Text("app=ledger")
			put(m, pod, t0, attrs)
			node := newID(kube.KindNode, "", "n2")
			put(m, node, t0, map[string]inventory.Value{
				kube.AttrUnschedulable: inventory.Bool(true),
			})
			link(m, pod, inventory.RunsOn, node)
			id := budget(m, "ledger", "app=ledger", 1)
			put(m, id, t0, map[string]inventory.Value{
				kube.AttrSelector:           inventory.Text("app=ledger"),
				kube.AttrExpectedPods:       inventory.Number(1),
				kube.AttrDisruptionsAllowed: inventory.Number(0),
			})

			late := evaluate(Budget{}, m, t0.Add(11*time.Minute), id, nil)

			assert.Empty(t, late.Findings)
		})
	}
}
