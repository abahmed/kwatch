package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func stuckDeletion(severity detection.Severity) detection.Finding {
	return sig(entity(kube.KindSecret, "old-token"),
		reasons.StuckDeleting, severity)
}

// A stuck deletion announced as a warning before it was known to be a
// leftover is judged again after a restart: once the detector reports it
// as housekeeping, the restored Notify incident falls to the digest and
// its thread hears the cause once.
func TestRestoredStuckDeletionFallsToDigestAsALeftover(t *testing.T) {
	src := newRig(t, Config{})
	announced(t, src, stuckDeletion(detection.Warning))
	assert.Equal(t, Notify, src.only().Tier)

	dst := newRig(t, Config{})
	grace := at(10 * time.Minute)
	dst.m.Restore(src.m.Export(), grace)
	dst.raise(at(time.Minute), stuckDeletion(detection.Info))
	wantNone(t, dst.tick(grace.Add(-time.Second)))

	wantNone(t, dst.tick(grace))
	ds := dst.tick(grace.Add(DefaultReviseSettle))
	wantAction(t, ds, Update, ReasonCauseRevised)
	assert.Equal(t, Digest, ds[0].Incident.Tier)
}

// A stuck deletion that is still a warning keeps its tier.
func TestRestoredStuckDeletionStaysNotifyWhileItBlocksSomething(t *testing.T) {
	src := newRig(t, Config{})
	announced(t, src, stuckDeletion(detection.Warning))

	dst := newRig(t, Config{})
	grace := at(10 * time.Minute)
	dst.m.Restore(src.m.Export(), grace)
	dst.raise(at(time.Minute), stuckDeletion(detection.Warning))

	wantNone(t, dst.tick(grace))
	wantNone(t, dst.tick(grace.Add(DefaultReviseSettle)))
	assert.Equal(t, Notify, dst.only().Tier)
}
