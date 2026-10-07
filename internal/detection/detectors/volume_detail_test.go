package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestVolumeInodesFindingWithPlentyOfBytes(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPVC, "mail", "spool")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrVolumeUsedPct:   inventory.Number(12),
		kube.AttrVolumeInodesPct: inventory.Number(97),
	})
	got := evaluate(VolumeUsage{}, m, t0, id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.VolumeInodesHigh, got[0].Reason)
	assert.Equal(t, detection.Critical, got[0].Severity)
	assert.Equal(t, "Volume has used over 95% of its inodes",
		got[0].Summary)
	assert.Equal(t, detection.ModeVolumeInodes, got[0].Mode)
}

func TestVolumeInodesWarningAndQuiet(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPVC, "mail", "spool")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrVolumeInodesPct: inventory.Number(86)})
	got := evaluate(VolumeUsage{}, m, t0, id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "over 85%")

	quiet := newTestModel()
	put(quiet, id, t0, map[string]inventory.Value{
		kube.AttrVolumeInodesPct: inventory.Number(40)})
	assert.Empty(t, evaluate(VolumeUsage{}, quiet, t0, id, nil).Findings)
}

func TestFullVolumeNamesItsSizeAndUsers(t *testing.T) {
	m := newTestModel()
	claim := newID(kube.KindPVC, "inventory", "pgdata")
	put(m, claim, t0, map[string]inventory.Value{
		kube.AttrVolumeUsedPct:       inventory.Number(96),
		kube.AttrVolumeUsedBytes:     inventory.Number(19.2 * (1 << 30)),
		kube.AttrVolumeCapacityBytes: inventory.Number(20 * (1 << 30)),
	})
	for _, name := range []string{"postgres-0", "postgres-1", "backup-1"} {
		pod := newID(kube.KindPod, "inventory", name)
		put(m, pod, t0, nil)
		link(m, pod, inventory.Mounts, claim)
		owner := name[:len(name)-2]
		set := newID(kube.KindStatefulSet, "inventory", owner)
		put(m, set, t0, nil)
		link(m, pod, inventory.OwnedBy, set)
	}
	got := evaluate(VolumeUsage{}, m, t0, claim, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, "19.2Gi of 20Gi",
		evidenceOf(got[0], detection.EvidenceVolumeSize))
	assert.Equal(t, "backup, postgres",
		evidenceOf(got[0], detection.EvidenceVolumeUsedBy))
}

func TestFullVolumeWithoutSizeOrUsersAddsNoDetail(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPVC, "inventory", "pgdata")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrVolumeUsedPct: inventory.Number(96)})
	got := evaluate(VolumeUsage{}, m, t0, id, nil).Findings
	require.Len(t, got, 1)
	assert.Len(t, got[0].Evidence, 1, "only the used share")
}
