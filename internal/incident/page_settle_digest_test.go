package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Incidents of the digest tier never speak on their own, so they are not
// a burst: a page settling beside three of them still goes out at the
// page settle, not after the full one.
func TestPageIsNotDelayedByDigestIncidentsSettlingBesideIt(t *testing.T) {
	r := newRig(t, Config{})
	var quiet []detection.Finding
	for _, name := range []string{"a", "b", "c"} {
		quiet = append(quiet, sig(entity(kube.KindService, "svc-"+name),
			reasons.ServiceNoEndpoints, detection.Info))
	}
	r.raise(at(0), quiet...)
	r.raise(at(time.Second), nodeDown())
	require.Equal(t, Digest,
		r.of(entity(kube.KindService, "svc-a")).Tier, "setup: quiet tier")

	ds := r.tick(at(time.Second + DefaultPageSettle))

	wantAction(t, ds, Announce, ReasonSettled)
	assert.Equal(t, Page, ds[0].Incident.Tier)
}
