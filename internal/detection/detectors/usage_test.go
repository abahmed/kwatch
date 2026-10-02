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

func TestVolumeUsageThresholdAndFill(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPVC, "default", "data")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrVolumeUsedPct: inventory.Number(90),
		kube.AttrVolumeFillETA: inventory.Number(3600),
	})
	got := evaluate(VolumeUsage{}, m, t0, id, nil).Findings
	require.Len(t, got, 2)
	assert.Equal(t, reasons.VolumeUsageHigh, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, reasons.VolumeFillingUp, got[1].Reason)
	assert.Equal(t, detection.Critical, got[1].Severity)
	assert.NotEmpty(t, got[1].Summary)
}

func TestVolumeUsageFillBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		eta      time.Duration
		fires    bool
		severity detection.Severity
	}{
		{"critical at 6h", 6 * time.Hour, true, detection.Critical},
		{"warning just past 6h", 6*time.Hour + time.Second, true,
			detection.Warning},
		{"warning at 24h", 24 * time.Hour, true, detection.Warning},
		{"quiet past 24h", 24*time.Hour + time.Second, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			id := newID(kube.KindPVC, "default", "data")
			put(m, id, t0, map[string]inventory.Value{
				kube.AttrVolumeFillETA: inventory.Number(tt.eta.Seconds()),
			})
			got := evaluate(VolumeUsage{}, m, t0, id, nil).Findings
			if !tt.fires {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, tt.severity, got[0].Severity)
		})
	}
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

func TestVolumeUsageIgnoresInvalidFillEstimates(t *testing.T) {
	for _, seconds := range []float64{-9.2e9, 1e12} {
		m := newTestModel()
		id := newID(kube.KindPVC, "default", "data")
		put(m, id, t0, map[string]inventory.Value{
			kube.AttrVolumeFillETA: inventory.Number(seconds),
		})
		got := evaluate(VolumeUsage{}, m, t0, id, nil).Findings
		assert.Empty(t, got, "estimate %v must not raise a finding", seconds)
	}
}
