package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestNodeUsageThresholds(t *testing.T) {
	tests := []struct {
		name     string
		attrs    map[string]knowledge.Value
		reason   string
		severity signal.Severity
	}{
		{"disk warning", map[string]knowledge.Value{
			kube.AttrFSUsedPct: knowledge.Number(85)},
			constant.ReasonNodeFilesystemHigh, signal.Warning},
		{"disk critical", map[string]knowledge.Value{
			kube.AttrFSUsedPct: knowledge.Number(95)},
			constant.ReasonNodeFilesystemCritical, signal.Critical},
		{"inodes warning", map[string]knowledge.Value{
			kube.AttrInodesUsedPct: knowledge.Number(90)},
			constant.ReasonNodeInodesHigh, signal.Warning},
		{"inodes critical", map[string]knowledge.Value{
			kube.AttrInodesUsedPct: knowledge.Number(99)},
			constant.ReasonNodeInodesCritical, signal.Critical},
		{"memory stall", map[string]knowledge.Value{
			kube.AttrMemoryPSI: knowledge.Number(20)},
			constant.ReasonNodePSIHigh, signal.Warning},
		{"cpu stall", map[string]knowledge.Value{
			kube.AttrCPUPSI: knowledge.Number(30)},
			constant.ReasonNodePSIHigh, signal.Warning},
		{"io stall", map[string]knowledge.Value{
			kube.AttrIOPSI: knowledge.Number(50)},
			constant.ReasonNodePSIHigh, signal.Warning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			id := newID(kube.KindNode, "", "n1")
			put(m, id, t0, tt.attrs)
			got := evaluate(NodeUsage{}, m, t0, id, nil).Signals
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
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrFSUsedPct:     knowledge.Number(84.9),
		kube.AttrInodesUsedPct: knowledge.Number(10),
		kube.AttrMemoryPSI:     knowledge.Number(19.9),
	})
	assert.Empty(t, evaluate(NodeUsage{}, m, t0, id, nil).Signals)
	assert.Equal(t, "node-usage", NodeUsage{}.Name())
	assert.Equal(t, []knowledge.Kind{kube.KindNode}, NodeUsage{}.Kinds())
}

func TestVolumeUsageThresholdAndFill(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPVC, "default", "data")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrVolumeUsedPct: knowledge.Number(90),
		kube.AttrVolumeFillETA: knowledge.Number(3600),
	})
	got := evaluate(VolumeUsage{}, m, t0, id, nil).Signals
	require.Len(t, got, 2)
	assert.Equal(t, constant.ReasonVolumeUsageHigh, got[0].Reason)
	assert.Equal(t, signal.Warning, got[0].Severity)
	assert.Equal(t, constant.ReasonVolumeFillingUp, got[1].Reason)
	assert.Equal(t, signal.Critical, got[1].Severity)
	assert.NotEmpty(t, got[1].Summary)
}

func TestVolumeUsageFillBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		eta      time.Duration
		fires    bool
		severity signal.Severity
	}{
		{"critical at 6h", 6 * time.Hour, true, signal.Critical},
		{"warning just past 6h", 6*time.Hour + time.Second, true,
			signal.Warning},
		{"warning at 24h", 24 * time.Hour, true, signal.Warning},
		{"quiet past 24h", 24*time.Hour + time.Second, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			id := newID(kube.KindPVC, "default", "data")
			put(m, id, t0, map[string]knowledge.Value{
				kube.AttrVolumeFillETA: knowledge.Number(tt.eta.Seconds()),
			})
			got := evaluate(VolumeUsage{}, m, t0, id, nil).Signals
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
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrVolumeUsedPct: knowledge.Number(50),
	})
	assert.Empty(t, evaluate(VolumeUsage{}, m, t0, id, nil).Signals)
	assert.Equal(t, "volume-usage", VolumeUsage{}.Name())
	assert.Equal(t, []knowledge.Kind{kube.KindPVC}, VolumeUsage{}.Kinds())
}

func TestUsageUpperEmpty(t *testing.T) {
	assert.Equal(t, "", upper(""))
	assert.Equal(t, "Disk", upper("disk"))
}
