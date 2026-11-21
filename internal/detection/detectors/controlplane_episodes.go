package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

const (
	// outageEpisodes is how many separate outages of one control plane
	// service, each too short to be a finding alone, make a pattern.
	outageEpisodes = 3
	// outageWindow is how far back the first of those outages may lie.
	outageWindow = 15 * time.Minute
	// outageRapid is the span of those outages below which the service
	// is in a restart loop.
	outageRapid = 5 * time.Minute
)

// episodeKey names the tracked start of the n-th latest outage.
func episodeKey(n int) string { return "outage-start/" + strconv.Itoa(n) }

// outageStarts returns the starts of the latest outages, newest first,
// and keeps them remembered. A start is zero when none is known.
func outageStarts(ctx detection.Context) [outageEpisodes]time.Time {
	var starts [outageEpisodes]time.Time
	for n := range starts {
		prev, found := ctx.Track(episodeKey(n), time.Time{})
		if found {
			starts[n] = prev
			ctx.Track(episodeKey(n), prev)
		}
	}
	return starts
}

// recurringOutage notes a new outage when the probe turns unhealthy and
// returns a finding while the latest outageEpisodes outages all lie
// within outageWindow: a service that goes down and comes back again
// and again never stays down long enough for the sustained check, yet
// it is failing. since is when the current unhealthy state began.
func recurringOutage(
	ctx detection.Context, e inventory.Entity, unhealthy bool,
	since time.Time,
) []detection.Finding {
	starts := outageStarts(ctx)
	if unhealthy {
		if !ctx.Ongoing("outage-ongoing") {
			for n := outageEpisodes - 1; n > 0; n-- {
				starts[n] = starts[n-1]
			}
			starts[0] = since
			for n, at := range starts {
				ctx.Track(episodeKey(n), at)
			}
		}
		ctx.Onset("outage-ongoing", ctx.Now)
	}
	oldest := starts[outageEpisodes-1]
	if oldest.IsZero() || ctx.Now.Sub(oldest) > outageWindow {
		return nil
	}
	ctx.RecheckAfter(outageWindow - ctx.Now.Sub(oldest))
	reason, what := serviceReason(e.ID)
	// Outages that come within minutes of each other are a restart
	// loop; spread over a quarter of an hour they are a warning.
	severity := detection.Warning
	if starts[0].Sub(oldest) <= outageRapid {
		severity = detection.Critical
	}
	return []detection.Finding{{
		Reason: reason, Severity: severity, Since: oldest,
		Summary: what + " has been down " + strconv.Itoa(outageEpisodes) +
			" times in the last " +
			strconv.Itoa(int(outageWindow.Minutes())) +
			" minutes",
		Evidence: errorEvidence(e),
	}}
}
