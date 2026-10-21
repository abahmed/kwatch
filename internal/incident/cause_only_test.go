package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A finding that fires before anything failed (a volume nearly full, an
// autoscaler at its maximum) is a cause, not news: alone it says nothing,
// not even in the digest, however long it lasts.
func TestCauseOnlyIncidentIsSilent(t *testing.T) {
	for _, reason := range []string{reasons.VolumeUsageHigh,
		reasons.VolumeInodesHigh, reasons.HPAMaxedOut} {
		t.Run(reason, func(t *testing.T) {
			r := newRig(t, Config{})
			r.raise(at(0), sig(entity(kube.KindPVC, "data"), reason,
				detection.Critical))

			wantNone(t, r.tick(at(DefaultSettle)))
			wantNone(t, r.tick(at(24*time.Hour)))
			assert.Equal(t, Silent, r.only().Tier)
			assert.True(t, r.only().Announced.IsZero())
		})
	}
}

// Once a failure joins, the incident is announced with the cause as its
// root at the next tick: it settled long ago, like a digest incident that
// is promoted.
func TestCauseOnlyIncidentSpeaksWhenAFailureJoins(t *testing.T) {
	r := newRig(t, Config{})
	claim := entity(kube.KindPVC, "data")
	pod := entity(kube.KindPod, "db")
	r.cause(pod, claim, "the volume is full")
	r.raise(at(0), sig(claim, reasons.VolumeUsageHigh, detection.Critical))
	wantNone(t, r.tick(at(time.Hour)))

	joined := time.Hour + time.Minute
	r.raise(at(joined), sig(pod, reasons.CrashLoopBackOff,
		detection.Warning))

	ds := r.tick(at(joined))
	wantAction(t, ds, Announce, ReasonSettled)
	assert.Equal(t, claim, ds[0].Incident.Root)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
}

// A reason that is not cause-only keeps its tier: only the listed
// findings are silenced.
func TestCauseOnlyDoesNotSilenceOtherReasons(t *testing.T) {
	p := incidentOf(entity(kube.KindPVC, "data"), nil,
		sig(entity(kube.KindPVC, "data"), reasons.VolumeUsageHigh,
			detection.Critical),
		sig(entity(kube.KindPod, "db"), reasons.CrashLoopBackOff,
			detection.Warning))
	assert.Equal(t, Notify, tier(p))
}

// A node condition nobody has a detector for is not silent once it has
// lasted: it is one line in the digest. Pods that fail on the node make
// it the cause of an ordinary incident.
func TestNodeConditionIncidentIsOneDigestLine(t *testing.T) {
	node := entity(kube.KindNode, "worker-4")
	reason := reasons.NodeConditionPrefix + "ReadonlyFilesystem"

	alone := incidentOf(node, nil, sig(node, reason, detection.Warning))
	failing := incidentOf(node, nil, sig(node, reason, detection.Warning),
		sig(entity(kube.KindPod, "db"), reasons.CrashLoopBackOff,
			detection.Warning))

	assert.Equal(t, Digest, tier(alone))
	assert.Equal(t, Notify, tier(failing))
}
