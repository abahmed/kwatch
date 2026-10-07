package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestDiskFullWithInodesGoneBlamesTheClaim: a write that fails with "No
// space left on device" while the claim has bytes to spare is the claim
// out of inodes, and it is worded as that mode.
func TestDiskFullWithInodesGoneBlamesTheClaim(t *testing.T) {
	cases := map[string]struct {
		used, inodes float64
		want         detection.Mode
		full         bool
	}{
		"inodes gone, bytes free": {12, 98, detection.ModeVolumeInodes, true},
		"both gone":               {97, 99, detection.ModeVolumeFull, true},
		"neither":                 {30, 40, "", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			effect := fullClaimCase(f, "Bound", "write /spool/x: "+
				"No space left on device")
			claim := inventory.CoreID(kube.KindPVC, "data", "pgdata")
			f.observe(claim, map[string]inventory.Value{
				kube.AttrPhase:           inventory.Text("Bound"),
				kube.AttrVolumeUsedPct:   inventory.Number(c.used),
				kube.AttrVolumeInodesPct: inventory.Number(c.inodes)})
			v := newView(f.snapshot())
			modes := v.virtualModes(claim, effect, LinkMounts)
			if (len(modes) > 0) != c.full {
				t.Fatalf("full = %v, want %v", len(modes) > 0, c.full)
			}
			if c.full && modes[0].mode != c.want {
				t.Fatalf("mode = %s, want %s", modes[0].mode, c.want)
			}
		})
	}
}
