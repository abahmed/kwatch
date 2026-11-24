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

// After a restart kwatch stores a budget's attributes anew; a budget the
// controller found empty long ago is dated by its DisruptionAllowed
// condition, not by that first sight, and only waits for the digest.
func TestBudgetSelectingNothingAfterRestartKeepsItsAge(t *testing.T) {
	boot := t0.Add(447 * 24 * time.Hour)
	emptied := t0
	m := newTestModel()
	labelledPod(m, "web-1", "app=web")
	id := newID(kube.KindPDB, "ns", "api")
	put(m, id, boot, conditionAttrs(map[string]inventory.Value{
		kube.AttrSelector:           inventory.Text("app=api"),
		kube.AttrExpectedPods:       inventory.Number(0),
		kube.AttrDisruptionsAllowed: inventory.Number(0),
	}, "DisruptionAllowed", "False", "InsufficientPods", emptied))

	eval := evaluate(Budget{}, m, boot.Add(time.Minute), id, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, reasons.PdbSelectsNothing, f.Reason)
	assert.True(t, emptied.Equal(f.Since), "the cluster's own record")
	assert.True(t, detection.DigestOnly(f.Reason),
		"a budget that selects nothing never interrupts")
}

// A budget whose selector matches the template of a workload scaled to
// zero protects that workload when it runs again.
func TestBudgetOfWorkloadScaledToZeroIsFine(t *testing.T) {
	m := newTestModel()
	id := budget(m, "api", "app=api", 0)
	put(m, newID(kube.KindDeployment, "ns", "api"), t0,
		map[string]inventory.Value{
			kube.AttrReplicas:       inventory.Number(0),
			kube.AttrTemplateLabels: inventory.Text("app=api"),
		})

	eval := evaluate(Budget{}, m, t0.Add(time.Hour), id, nil)

	assert.Empty(t, eval.Findings)
}

// A budget of minAvailable 1 on one replica never allowed a disruption:
// its DisruptionAllowed condition has been False for a year. When kwatch
// sees its last pod go, the budget is empty from then, not for a year.
func TestBudgetEmptiedWhileWatchedIsDatedFromTheEmptying(t *testing.T) {
	strict := t0.Add(-365 * 24 * time.Hour)
	emptied := t0.Add(time.Hour)
	m := newTestModel()
	id := newID(kube.KindPDB, "ns", "api")
	attrs := func(expected float64) map[string]inventory.Value {
		return conditionAttrs(map[string]inventory.Value{
			kube.AttrSelector:           inventory.Text("app=api"),
			kube.AttrExpectedPods:       inventory.Number(expected),
			kube.AttrDisruptionsAllowed: inventory.Number(0),
		}, "DisruptionAllowed", "False", "InsufficientPods", strict)
	}
	put(m, id, t0, attrs(1))
	put(m, id, emptied, attrs(0))

	eval := evaluate(Budget{}, m, emptied.Add(time.Minute), id, nil)
	assert.Empty(t, eval.Findings, "empty for a minute, not a year")

	eval = evaluateAfterGrace(Budget{}, m, emptied.Add(time.Minute),
		DefaultBudgetBlocked, id, nil)
	require.Len(t, eval.Findings, 1)
	assert.True(t, emptied.Equal(eval.Findings[0].Since),
		"dated from the emptying")
}
