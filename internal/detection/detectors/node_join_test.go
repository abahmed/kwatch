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

var joinNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// joiningNode builds a node created joinedAgo before joinNow whose
// Ready condition last changed at transition.
func joiningNode(
	m *inventory.Model, joinedAgo time.Duration, status, reason string,
	transition time.Time,
) inventory.EntityID {
	id := newID(kube.KindNode, "", "n-new")
	created := joinNow.Add(-joinedAgo)
	attrs := conditionAttrs(map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(created),
		kube.AttrReady:   inventory.Bool(status == "True"),
	}, "Ready", status, reason, transition)
	observeEntity(m, id, joinNow.Add(-joinedAgo), attrs)
	return id
}

func evalNode(
	m *inventory.Model, id inventory.EntityID,
) detection.Evaluation {
	registry := detection.NewRegistry(nil, NewNode(0))
	return registry.Evaluate(m, joinNow, id)
}

func TestNodeNeverReadyWaitsForBootWindowFromCreation(t *testing.T) {
	m := newTestModel()
	created := joinNow.Add(-5 * time.Minute)
	id := joiningNode(m, 5*time.Minute, "False", "KubeletNotReady",
		created.Add(2*time.Second))

	got := evalNode(m, id)

	assert.Empty(t, got.Findings)
	assert.Equal(t, kube.BootWindow-5*time.Minute, got.RecheckAfter)
}

func TestNodeNeverReadyIsReportedAfterBootWindow(t *testing.T) {
	m := newTestModel()
	created := joinNow.Add(-12 * time.Minute)
	id := joiningNode(m, 12*time.Minute, "False", "KubeletNotReady",
		created.Add(2*time.Second))

	got := evalNode(m, id).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.NodeNotReady, got[0].Reason)
	assert.Equal(t, detection.Critical, got[0].Severity)
	assert.Contains(t, got[0].Summary,
		"has not been Ready since it joined 12m ago")
}

func TestNodeReadyThenNotReadyKeepsNinetySecondRule(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n-new")
	created := joinNow.Add(-8 * time.Minute)
	ready := conditionAttrs(map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(created),
		kube.AttrReady:   inventory.Bool(true),
	}, "Ready", "True", "KubeletReady", created.Add(time.Minute))
	observeEntity(m, id, created.Add(time.Minute), ready)
	lost := joinNow.Add(-3 * time.Minute)
	down := conditionAttrs(map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(created),
		kube.AttrReady:   inventory.Bool(false),
	}, "Ready", "False", "KubeletNotReady", lost)
	observeEntity(m, id, lost, down)

	got := evalNode(m, id).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.NodeNotReady, got[0].Reason)
	assert.Contains(t, got[0].Summary, "NotReady for 3m")
}

// A node kwatch never saw Ready whose Ready condition changed long
// after it joined was Ready in between: the usual rule applies.
func TestNodeLateNotReadyTransitionKeepsNinetySecondRule(t *testing.T) {
	m := newTestModel()
	lost := joinNow.Add(-3 * time.Minute)
	id := joiningNode(m, 8*time.Minute, "False", "KubeletNotReady", lost)

	got := evalNode(m, id).Findings

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "NotReady for 3m")
}

func TestNodeHeartbeatStaleWaitsWhileNodeNeverReady(t *testing.T) {
	m := newTestModel()
	created := joinNow.Add(-5 * time.Minute)
	id := joiningNode(m, 5*time.Minute, "Unknown", "NodeStatusUnknown",
		created.Add(time.Minute))

	got := evalNode(m, id)

	assert.Empty(t, got.Findings)
	assert.Equal(t, kube.BootWindow-5*time.Minute, got.RecheckAfter)
}

func TestNodeHeartbeatStaleIsReportedAfterBootWindow(t *testing.T) {
	m := newTestModel()
	created := joinNow.Add(-12 * time.Minute)
	id := joiningNode(m, 12*time.Minute, "Unknown", "NodeStatusUnknown",
		created.Add(time.Minute))

	got := evalNode(m, id).Findings

	assert.ElementsMatch(t,
		[]string{reasons.NodeNotReady, reasons.NodeHeartbeatStale},
		findingReasons(got))
}

func TestNodeWithoutCreationTimeKeepsNinetySecondRule(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n-old")
	since := joinNow.Add(-2 * time.Minute)
	observeEntity(m, id, since, conditionAttrs(nil, "Ready", "False",
		"KubeletNotReady", since))

	got := evalNode(m, id).Findings

	assert.Equal(t, []string{reasons.NodeNotReady}, findingReasons(got))
}
