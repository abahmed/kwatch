package kube

import "github.com/abahmed/kwatch/internal/inventory"

// Attribute names of a claim's size and inode use, written beside
// AttrVolumeUsedPct from the same kubelet reading.
const (
	AttrVolumeInodesPct     = "volume.inodes.used.pct"
	AttrVolumeUsedBytes     = "volume.used.bytes"
	AttrVolumeCapacityBytes = "volume.capacity.bytes"
)

// setVolumeSize records how many bytes of a claim are used and how many
// it holds, and the share of its inodes in use. A filesystem can run
// out of inodes with bytes to spare, and every write then fails with
// "No space left on device". The kubelet leaves inodes out for volumes
// that have none to count (some network filesystems); then no
// attribute is written.
func setVolumeSize(attrs map[string]inventory.Value, v volumeUsage) {
	if v.CapacityBytes > 0 {
		attrs[AttrVolumeUsedBytes] = inventory.Number(float64(v.UsedBytes))
		attrs[AttrVolumeCapacityBytes] = inventory.Number(
			float64(v.CapacityBytes))
	}
	// A free count above the total is a bad reading, not a negative use.
	if v.Inodes > 0 && v.InodesFree <= v.Inodes {
		attrs[AttrVolumeInodesPct] = inventory.Number(
			100 * float64(v.Inodes-v.InodesFree) / float64(v.Inodes))
	}
}
