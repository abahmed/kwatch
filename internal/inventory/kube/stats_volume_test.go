package kube

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func volumeAttrs(t *testing.T, summary string) map[string]inventory.Value {
	t.Helper()
	var s statsSummary
	require.NoError(t, json.Unmarshal([]byte(summary), &s))
	volumes := s.volumes()
	require.Len(t, volumes, 1)
	attrs := map[string]inventory.Value{}
	setVolumeSize(attrs, volumes[0])
	return attrs
}

const volumeSummary = `{"pods":[{"podRef":{"name":"p","namespace":"ns"},
"volume":[{"usedBytes":10,"capacityBytes":100,"inodes":%s,
"inodesFree":%s,"pvcRef":{"name":"data","namespace":"ns"}}]}]}`

func TestVolumeSizeAndInodes(t *testing.T) {
	attrs := volumeAttrs(t, fmt.Sprintf(volumeSummary, "1000", "50"))
	pct, _ := attrs[AttrVolumeInodesPct].AsNumber()
	assert.InDelta(t, 95, pct, 0.001)
	used, _ := attrs[AttrVolumeUsedBytes].AsNumber()
	capacity, _ := attrs[AttrVolumeCapacityBytes].AsNumber()
	assert.InDelta(t, 10, used, 0.001)
	assert.InDelta(t, 100, capacity, 0.001)
}

func TestVolumeInodesAreSkippedWhenUncountable(t *testing.T) {
	// A volume with no inodes to count, and a free count above the total.
	for _, free := range []string{"0", "2000"} {
		total := "0"
		if free == "2000" {
			total = "1000"
		}
		attrs := volumeAttrs(t, fmt.Sprintf(volumeSummary, total, free))
		_, ok := attrs[AttrVolumeInodesPct]
		assert.False(t, ok, "inodes %s free %s", total, free)
	}
}
