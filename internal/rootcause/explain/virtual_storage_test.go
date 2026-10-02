package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// fullClaimCase is a database whose bound claim names a volume that is
// not listed, crashing with text.
func fullClaimCase(f *fixture, phase, text string) inventory.EntityID {
	pod := f.workload("data", "db", 1)[0]
	claim := inventory.CoreID(kube.KindPVC, "data", "pgdata")
	volume := inventory.CoreID(kube.KindPV, "", "pv-1")
	f.observe(claim, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text(phase)})
	f.relate(claim, inventory.References, volume)
	f.relate(pod, inventory.Mounts, claim)
	f.fail(containerOf(pod), "CrashLoop", failingH, 2, text)
	return containerOf(pod)
}

func TestClaimFullBlamesTheClaim(t *testing.T) {
	f := newFixture(t)
	effect := fullClaimCase(f, "Bound", "could not write to file "+
		"\"pg_wal/xlogtemp.31\": No space left on device")
	c := requireCause(t, f.explain(), effect,
		"persistentvolumeclaim/data/pgdata")
	if c.Row != "claim-not-usable" || c.Mode != detection.ModeVolumeFull {
		t.Fatalf("cause = %s (%s), want claim-not-usable (%s)", c.Row,
			c.Mode, detection.ModeVolumeFull)
	}
}

// TestBoundVolumeIsNeverMissing: a Bound claim proves its volume exists,
// even when kwatch does not list it, so the volume is not blamed.
func TestBoundVolumeIsNeverMissing(t *testing.T) {
	cases := map[string]struct {
		phase   string
		missing bool
	}{
		"bound claim":   {"Bound", false},
		"pending claim": {"Pending", true},
	}
	volume := inventory.CoreID(kube.KindPV, "", "pv-1")
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			effect := fullClaimCase(f, c.phase, "exit status 1: the "+
				"database cannot start")
			v := newView(f.snapshot())
			modes := v.virtualModes(volume, effect, LinkUses)
			if missing := len(modes) > 0 &&
				modes[0].mode == ModeMissing; missing != c.missing {
				t.Fatalf("modes = %+v, want missing %v", modes, c.missing)
			}
		})
	}
}

func TestDiskFullNeedsTheMountedClaim(t *testing.T) {
	f := newFixture(t)
	effect := fullClaimCase(f, "Bound", "write /data/x: no space left "+
		"on device")
	v := newView(f.snapshot())
	claim := inventory.CoreID(kube.KindPVC, "data", "pgdata")
	if modes := v.virtualModes(claim, effect, LinkUses); len(modes) != 0 {
		t.Fatalf("a claim not mounted through this link was full: %+v",
			modes)
	}
	if modes := v.virtualModes(claim, effect, LinkMounts); len(modes) != 1 {
		t.Fatalf("the mounted claim must be full, got %+v", modes)
	}
}

// twoClaimCase is a pod mounting the claims data and logs, crashing
// with text, with the given used percentages (negative: no stats).
func twoClaimCase(
	f *fixture, text string, dataUsed, logsUsed float64,
) (effect, data, logs inventory.EntityID) {
	pod := f.workload("shop", "a", 1)[0]
	data = inventory.CoreID(kube.KindPVC, "shop", "data")
	logs = inventory.CoreID(kube.KindPVC, "shop", "logs")
	for claim, used := range map[inventory.EntityID]float64{
		data: dataUsed, logs: logsUsed} {
		attrs := map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Bound")}
		if used >= 0 {
			attrs[kube.AttrVolumeUsedPct] = inventory.Number(used)
		}
		f.observe(claim, attrs)
	}
	f.relate(pod, inventory.Mounts, data, logs)
	f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 2, text)
	return containerOf(pod), data, logs
}

// TestExplainDiskFullNeedsEvidenceForTheClaim: "no space left on
// device" alone does not say which of two mounted claims is full.
func TestExplainDiskFullNeedsEvidenceForTheClaim(t *testing.T) {
	f := newFixture(t)
	effect, _, _ := twoClaimCase(f, "write /tmp/x: no space left on "+
		"device", -1, -1)
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Root.Kind == kube.KindPVC {
		t.Fatalf("%s blamed on %s with nothing naming it", effect, c.Root)
	}
}

func TestDiskFullClaimEvidence(t *testing.T) {
	cases := map[string]struct {
		text               string
		dataUsed, logsUsed float64
		dataFull, logsFull bool
	}{
		"named in the path": {"write /var/lib/data/x: no space left " +
			"on device", -1, -1, true, false},
		"usage near full": {"write /tmp/x: no space left on device",
			97, 12, true, false},
		"usage low despite the name": {"write /data/x: no space left " +
			"on device", 40, -1, false, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			effect, data, logs := twoClaimCase(f, c.text, c.dataUsed,
				c.logsUsed)
			v := newView(f.snapshot())
			full := func(claim inventory.EntityID) bool {
				return len(v.virtualModes(claim, effect, LinkMounts)) > 0
			}
			if full(data) != c.dataFull || full(logs) != c.logsFull {
				t.Fatalf("full: data %v logs %v, want %v %v", full(data),
					full(logs), c.dataFull, c.logsFull)
			}
		})
	}
}

// TestDiskFullSkipsClaimWithRoomLeft: usage stats below the threshold
// clear a lone mounted claim of a full disk.
func TestDiskFullSkipsClaimWithRoomLeft(t *testing.T) {
	f := newFixture(t)
	effect := fullClaimCase(f, "Bound", "no space left on device")
	claim := inventory.CoreID(kube.KindPVC, "data", "pgdata")
	f.observe(claim, map[string]inventory.Value{
		kube.AttrPhase:         inventory.Text("Bound"),
		kube.AttrVolumeUsedPct: inventory.Number(30)})
	v := newView(f.snapshot())
	if modes := v.virtualModes(claim, effect, LinkMounts); len(modes) != 0 {
		t.Fatalf("a claim 30%% used was full: %+v", modes)
	}
}

func TestContainsWordRespectsBoundaries(t *testing.T) {
	cases := []struct {
		text, word string
		want       bool
	}{
		{"write to /data/pgdata/x", "pgdata", true},
		{"pgdata", "pgdata", true},
		{"pgdata-2 is full", "pgdata", false},
		{"xpgdata is full", "pgdata", false},
		{"xpgdata and pgdata", "pgdata", true},
		{"anything", "", false},
	}
	for _, tc := range cases {
		if got := containsWord(tc.text, tc.word); got != tc.want {
			t.Errorf("containsWord(%q, %q) = %v", tc.text, tc.word, got)
		}
	}
}
