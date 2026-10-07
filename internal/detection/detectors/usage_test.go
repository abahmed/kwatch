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

func TestNodeUsageThresholds(t *testing.T) {
	tests := []struct {
		name     string
		attrs    map[string]inventory.Value
		reason   string
		severity detection.Severity
	}{
		{"disk warning", map[string]inventory.Value{
			kube.AttrFSUsedPct: inventory.Number(85)},
			reasons.NodeFilesystemHigh, detection.Warning},
		{"disk critical", map[string]inventory.Value{
			kube.AttrFSUsedPct: inventory.Number(95)},
			reasons.NodeFilesystemHigh, detection.Critical},
		{"inodes warning", map[string]inventory.Value{
			kube.AttrInodesUsedPct: inventory.Number(90)},
			reasons.NodeInodesHigh, detection.Warning},
		{"inodes critical", map[string]inventory.Value{
			kube.AttrInodesUsedPct: inventory.Number(99)},
			reasons.NodeInodesHigh, detection.Critical},
		{"memory stall", map[string]inventory.Value{
			kube.AttrMemoryPSI: inventory.Number(20)},
			reasons.NodePSIHigh, detection.Warning},
		{"cpu stall", map[string]inventory.Value{
			kube.AttrCPUPSI: inventory.Number(30)},
			reasons.NodePSIHigh, detection.Warning},
		{"io stall", map[string]inventory.Value{
			kube.AttrIOPSI: inventory.Number(50)},
			reasons.NodePSIHigh, detection.Warning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			id := newID(kube.KindNode, "", "n1")
			put(m, id, t0, tt.attrs)
			got := evaluate(NodeUsage{}, m, t0, id, nil).Findings
			require.Len(t, got, 1)
			assert.Equal(t, tt.reason, got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestNodeUsageQuietBelowThreshold(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n1")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrFSUsedPct:     inventory.Number(84.9),
		kube.AttrInodesUsedPct: inventory.Number(10),
		kube.AttrMemoryPSI:     inventory.Number(19.9),
	})
	assert.Empty(t, evaluate(NodeUsage{}, m, t0, id, nil).Findings)
	assert.Equal(t, "node-usage", NodeUsage{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindNode}, NodeUsage{}.Kinds())
}

func TestVolumeUsageThreshold(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPVC, "default", "data")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrVolumeUsedPct: inventory.Number(90),
	})
	got := evaluate(VolumeUsage{}, m, t0, id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.VolumeUsageHigh, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

func TestVolumeUsageQuiet(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPVC, "default", "data")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrVolumeUsedPct: inventory.Number(50),
	})
	assert.Empty(t, evaluate(VolumeUsage{}, m, t0, id, nil).Findings)
	assert.Equal(t, "volume-usage", VolumeUsage{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindPVC}, VolumeUsage{}.Kinds())
}

func TestUsageUpperEmpty(t *testing.T) {
	assert.Equal(t, "", upper(""))
	assert.Equal(t, "Disk", upper("disk"))
}

// Usage hovering around a level keeps one finding whose severity does
// not flip with every sample, and whose start does not move.
func TestNodeUsageHysteresisKeepsOneFinding(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n1")
	registry := detection.NewRegistry(nil, NodeUsage{})
	sample := func(i int, pct float64) detection.Evaluation {
		at := t0.Add(time.Duration(i) * time.Minute)
		put(m, id, at, map[string]inventory.Value{
			kube.AttrFSUsedPct: inventory.Number(pct)})
		return registry.Evaluate(m, at, id)
	}
	steps := []struct {
		pct  float64
		want detection.Severity
	}{
		{96, detection.Critical},
		{93, detection.Critical}, // within the hysteresis of 95
		{90, detection.Warning},  // clearly below 95
		{82, detection.Warning},  // within the hysteresis of 85
	}
	for i, step := range steps {
		got := sample(i, step.pct).Findings
		require.Len(t, got, 1, "sample %d", i)
		assert.Equal(t, reasons.NodeFilesystemHigh, got[0].Reason)
		assert.Equal(t, step.want, got[0].Severity, "sample %d", i)
		assert.Equal(t, t0, got[0].Since, "sample %d", i)
	}
	assert.Empty(t, sample(len(steps), 79).Findings)
	assert.Empty(t, sample(len(steps)+1, 82).Findings,
		"a level left must be crossed again")
}

func kubeletFailureNode(
	status string, failures float64, span time.Duration,
) (*inventory.Model, inventory.EntityID) {
	model := newTestModel()
	node := newID(kube.KindNode, "", "10-0-67-130")
	put(model, node, t0, map[string]inventory.Value{
		kube.AttrKubeletFailures:    inventory.Number(failures),
		kube.AttrKubeletFailureSpan: inventory.Number(span.Seconds())})
	setCondition(model, node, "Ready", status, "KubeletReady", "", t0)
	return model, node
}

func TestNodeUsageReportsAKubeletKwatchKeepsFailingToReach(t *testing.T) {
	model, node := kubeletFailureNode("True", 6, time.Hour)

	got := NodeUsage{}.Detect(testDetectorContext(model, t0),
		entityOf(model, node))

	require.Len(t, got, 1)
	assert.Equal(t, reasons.KubeletUnreachable, got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Equal(t, "kwatch could not reach the kubelet on node "+
		"10-0-67-130 6 times in the last 6 hours; node metrics for it are "+
		"missing.", got[0].Summary)
}

func TestNodeUsageIgnoresFewKubeletFailuresAndNotReadyNodes(t *testing.T) {
	for name, tc := range map[string]struct {
		status   string
		failures float64
		span     time.Duration
	}{
		"two_failures_are_a_blip":            {"True", 2, time.Hour},
		"not_ready_node_has_its_own_finding": {"False", 9, time.Hour},
		"failures_within_minutes_are_a_blip": {"True", 6, time.Minute},
		"cleared_once_the_kubelet_answers":   {"True", 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			model, node := kubeletFailureNode(tc.status, tc.failures, tc.span)

			got := NodeUsage{}.Detect(testDetectorContext(model, t0),
				entityOf(model, node))

			assert.Empty(t, got)
		})
	}
}

// When the kubelet cannot be read, the model forgets the disk figures
// after a few failed polls. The disk did not get emptier meanwhile, so
// the finding is held instead of resolving "healthy".
func TestNodeDiskFindingIsHeldWhileTheKubeletIsUnreachable(t *testing.T) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	put(m, node, t0, map[string]inventory.Value{
		kube.AttrFSUsedPct: inventory.Number(96)})
	registry := detection.NewRegistry(nil, NodeUsage{})
	first := registry.Evaluate(m, t0, node)
	require.Len(t, first.Findings, 1)

	put(m, node, t0.Add(time.Minute), map[string]inventory.Value{
		kube.AttrKubeletFailures: inventory.Number(3)})
	held := registry.Evaluate(m, t0.Add(time.Minute), node)
	require.Len(t, held.Findings, 1)
	assert.Equal(t, first.Findings[0].Reason, held.Findings[0].Reason)
	assert.Equal(t, first.Findings[0].Severity, held.Findings[0].Severity)
	assert.Equal(t, first.Findings[0].Since, held.Findings[0].Since)

	put(m, node, t0.Add(2*time.Minute), map[string]inventory.Value{
		kube.AttrKubeletFailures: inventory.Number(0)})
	cleared := registry.Evaluate(m, t0.Add(2*time.Minute), node)
	assert.Empty(t, cleared.Findings, "the kubelet answers; no reading")
}

// The hold is for a short outage of the kubelet. After half an hour the
// last reading is too old to keep claiming the disk is full.
func TestHeldDiskFindingIsCappedAndSaysItIsStale(t *testing.T) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	put(m, node, t0, map[string]inventory.Value{
		kube.AttrFSUsedPct: inventory.Number(96)})
	registry := detection.NewRegistry(nil, NodeUsage{})
	require.Len(t, registry.Evaluate(m, t0, node).Findings, 1)

	put(m, node, t0.Add(time.Minute), map[string]inventory.Value{
		kube.AttrKubeletFailures: inventory.Number(3)})
	held := registry.Evaluate(m, t0.Add(time.Minute), node)
	require.Len(t, held.Findings, 1)
	var stale string
	for _, e := range held.Findings[0].Evidence {
		if e.Label == "reading stale since" {
			stale = e.Value
		}
	}
	assert.Contains(t, stale, "UTC", "the stale time is named")

	for _, minutes := range []int{10, 20, 31, 32} {
		at := t0.Add(time.Duration(minutes) * time.Minute)
		held = registry.Evaluate(m, at, node)
	}
	assert.Empty(t, held.Findings, "held no longer than usageHoldMax")
}
