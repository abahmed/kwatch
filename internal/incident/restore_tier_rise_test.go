package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// restoredWeb announces a crash-looping web at the notify tier, then
// restarts kwatch ten minutes later: the incident is restored, its
// findings are raised again, and the restore grace runs for ten minutes.
func restoredWeb(
	t *testing.T, cfg Config,
) (*rig, detection.Finding, time.Time) {
	t.Helper()
	old := newRig(t, cfg)
	pod, _ := workload(old, "web")
	web := sig(pod, reasons.CrashLoopBackOff, detection.Warning)
	announced(t, old, web)

	restartAt := at(10 * time.Minute)
	fresh := newRig(t, cfg)
	fresh.model = old.model
	fresh.m.Restore(old.m.Export(), restartAt.Add(10*time.Minute))
	fresh.raise(restartAt, web)
	return fresh, web, restartAt
}

// A tier rise inside the restore grace is news. The grace swallows the
// restart's formula changes, not a real failure that got worse, and the
// page above all must not be lost.
func TestRestoreGraceDoesNotSwallowARiseToPage(t *testing.T) {
	cfg := Config{SeverityByReason: map[string]string{
		reasons.OOMKilled: "critical"}}
	for name, after := range map[string]time.Duration{
		"inside the grace": 2 * time.Minute,
		"after the grace":  12 * time.Minute,
	} {
		t.Run(name, func(t *testing.T) {
			r, web, restartAt := restoredWeb(t, cfg)
			wantNone(t, r.tick(restartAt.Add(time.Second)))

			oom := sig(web.Entity, reasons.OOMKilled, detection.Critical)
			r.raise(restartAt.Add(after), oom)
			var told []Decision
			for _, d := range []time.Duration{0, time.Minute} {
				told = append(told, r.tick(restartAt.Add(after+d))...)
			}

			require.Len(t, told, 1, "the rise is told once")
			assert.Equal(t, Update, told[0].Action)
			assert.Equal(t, Page, told[0].Incident.Tier)
		})
	}
}

// A restart on its own, with nothing worse, still says nothing: the
// fingerprint is adopted, so an upgrade costs no message.
func TestRestoreGraceStillAdoptsTheFingerprintOfAnUnchangedIncident(
	t *testing.T,
) {
	r, _, restartAt := restoredWeb(t, Config{})

	for now := time.Second; now <= 12*time.Minute; now += time.Minute {
		wantNone(t, r.tick(restartAt.Add(now)))
	}
	assert.Equal(t, Open, r.only().State)
}

// The rise told inside the grace is told once. The grace keeps adopting
// the fingerprint of the restored incident, and it must adopt it with the
// tier people were just told, not the saved one: a clamp to the saved tier
// would read as a second rise one MaterialGap later.
func TestRestoreGraceTellsARiseToPageOnce(t *testing.T) {
	cfg := Config{SeverityByReason: map[string]string{
		reasons.OOMKilled: "critical"}}
	r, web, restartAt := restoredWeb(t, cfg)
	wantNone(t, r.tick(restartAt.Add(time.Second)))
	oom := sig(web.Entity, reasons.OOMKilled, detection.Critical)
	r.raise(restartAt.Add(2*time.Minute), oom)

	var told []Decision
	risen := restartAt.Add(2 * time.Minute)
	span := 3 * MaterialGap
	for since := time.Duration(0); since <= span; since += time.Minute {
		told = append(told, r.tick(risen.Add(since))...)
	}

	require.Len(t, told, 1, "the rise is told once, not again later")
	assert.Equal(t, Page, told[0].Incident.Tier)
}
