package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func budget(m *inventory.Model, name, selector string,
	expected float64,
) inventory.EntityID {
	id := newID(kube.KindPDB, "ns", name)
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrSelector:           inventory.Text(selector),
		kube.AttrExpectedPods:       inventory.Number(expected),
		kube.AttrDisruptionsAllowed: inventory.Number(1),
	})
	return id
}

func labelledPod(
	m *inventory.Model, name, labels string,
) inventory.EntityID {
	id := newID(kube.KindPod, "ns", name)
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrLabels: inventory.Text(labels),
	})
	return id
}

func TestBudgetReportsSelectingNothing(t *testing.T) {
	m := newTestModel()
	labelledPod(m, "web-1", "app=web")
	id := budget(m, "api", "app=api", 0)

	early := evaluate(Budget{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)

	eval := evaluate(Budget{}, m, t0.Add(11*time.Minute), id, nil)
	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Budget.SelectsNothing", string(eval.Findings[0].Mode))
}

func TestBudgetSelectingPodsIsFine(t *testing.T) {
	m := newTestModel()
	labelledPod(m, "web-1", "app=web")
	id := budget(m, "web", "app=web", 1)

	eval := evaluate(Budget{}, m, t0.Add(time.Hour), id, nil)
	assert.Empty(t, eval.Findings)

	unsynced := func(kind inventory.Kind) bool {
		return kind != kube.KindPod
	}
	lone := budget(m, "none", "app=none", 0)
	eval = evaluate(Budget{}, m, t0.Add(time.Hour), lone, unsynced)
	assert.Empty(t, eval.Findings, "pods not watched")
}

func TestBudgetReportsOverlap(t *testing.T) {
	m := newTestModel()
	pod := labelledPod(m, "web-1", "app=web,tier=front")
	id := budget(m, "web", "app=web", 1)
	budget(m, "front", "tier=front", 1)
	budget(m, "other", "app=other", 0)
	warn(m, pod, t0, "MultiplePodDisruptionBudgets",
		"Pod belongs to multiple PodDisruptionBudgets")

	eval := evaluate(Budget{}, m, t0.Add(time.Minute), id, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, "Budget.Overlap", string(f.Mode))
	assert.Equal(t, detection.Failing, f.Health)
	assert.Contains(t, f.Summary, "also front")
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: "web-1",
		Value: "Pod belongs to multiple PodDisruptionBudgets"})
}

func TestBudgetReportsSyncFailed(t *testing.T) {
	m := newTestModel()
	labelledPod(m, "web-1", "app=web")
	id := budget(m, "web", "app=web", 1)
	setCondition(m, id, "DisruptionAllowed", "False", "SyncFailed",
		"found no controllers for pod", t0)

	eval := evaluate(Budget{}, m, t0.Add(5*time.Minute), id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Budget.SyncFailed", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Evidence, detection.Evidence{
		Label: "message", Value: "found no controllers for pod"})

	setCondition(m, id, "DisruptionAllowed", "False", "InsufficientPods",
		"", t0)
	eval = evaluate(Budget{}, m, t0.Add(5*time.Minute), id, nil)
	assert.Empty(t, eval.Findings)
}
