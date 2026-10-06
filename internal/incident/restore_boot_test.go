package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A known boot-time incident survives an 8 hour gap (nightly scale-down).
// The pods crash while they boot after the restart. That is boot noise
// again, measured from the restart and not from last night.
func TestKnownBootAfterLongGapStaysInTheDigest(t *testing.T) {
	r, _ := knownRig(t, crashSig)
	r.tick(at(DefaultSettle))
	recs := r.m.Export()

	fresh := newRig(t, Config{})
	_, pod := fresh.workloadRig(t, 2, 0, 3)
	gap := 8 * time.Hour
	fresh.m.Restore(recs, at(gap+10*time.Minute))
	fresh.raise(at(gap), crashSig(pod))

	wantNone(t, fresh.tick(at(gap+time.Minute)))
	assert.Equal(t, Digest, fresh.only().Tier)
}

// The same incident that is still crashing a boot window after the
// restart is not boot noise: it escalates, measured from the restart.
func TestKnownBootStillCrashingPastRestartBootWindowEscalates(t *testing.T) {
	r, _ := knownRig(t, crashSig)
	r.tick(at(DefaultSettle))
	recs := r.m.Export()

	fresh := newRig(t, Config{})
	_, pod := fresh.workloadRig(t, 2, 0, 3)
	gap := 8 * time.Hour
	fresh.m.Restore(recs, at(gap+10*time.Minute))
	fresh.raise(at(gap), crashSig(pod))
	wantNone(t, fresh.tick(at(gap+time.Minute)))

	for now := gap + 2*time.Minute; now < gap+time.Hour; now += time.Minute {
		fresh.raise(at(now), crashSig(pod))
		if ds := fresh.tick(at(now)); len(ds) > 0 {
			assert.GreaterOrEqual(t, now-gap, kube.BootWindow,
				"never before a boot window passed since the restart")
			assert.Equal(t, Notify, ds[0].Incident.Tier)
			return
		}
	}
	require.Fail(t, "a crash loop past the boot window never escalated")
}
