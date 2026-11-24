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
	// outageMinLength is how long an outage must last to count toward
	// a restart loop. A single lost probe reads as down until the next
	// probe, one probe interval (30s) later; the margin keeps the
	// probe's own timing from turning it into an outage.
	outageMinLength = 45 * time.Second
)

// episodeKey names the tracked start of the n-th latest outage.
func episodeKey(n int) string { return "outage-start/" + strconv.Itoa(n) }

// episodeEndKey names the tracked end of the n-th latest outage.
func episodeEndKey(n int) string { return "outage-end/" + strconv.Itoa(n) }

// outageTimes returns the times key names for the latest outages,
// newest first, and keeps them remembered. A time is zero when none is
// known.
func outageTimes(
	ctx detection.Context, key func(int) string,
) [outageEpisodes]time.Time {
	var times [outageEpisodes]time.Time
	for n := range times {
		prev, found := ctx.Track(key(n), time.Time{})
		if found {
			times[n] = prev
			ctx.Track(key(n), prev)
		}
	}
	return times
}

// shiftEpisodes makes room for a new outage that began at start: every
// tracked outage moves one place back, the oldest is dropped.
func shiftEpisodes(
	ctx detection.Context, starts, ends *[outageEpisodes]time.Time,
	start time.Time,
) {
	for n := outageEpisodes - 1; n > 0; n-- {
		starts[n], ends[n] = starts[n-1], ends[n-1]
	}
	starts[0], ends[0] = start, time.Time{}
	for n := range starts {
		ctx.Track(episodeKey(n), starts[n])
		ctx.Track(episodeEndKey(n), ends[n])
	}
}

// allLasted reports whether every tracked outage lasted outageMinLength.
// An outage without an end is still going on and lasts until now.
func allLasted(
	now time.Time, starts, ends [outageEpisodes]time.Time,
) bool {
	for n, start := range starts {
		end := ends[n]
		if end.IsZero() {
			end = now
		}
		if end.Sub(start) < outageMinLength {
			return false
		}
	}
	return true
}

// recurringOutage notes a new outage when the probe turns unhealthy, and
// its end when it turns healthy again, and returns a finding while the
// latest outageEpisodes outages all lie within outageWindow: a service
// that goes down and comes back again and again never stays down long
// enough for the sustained check, yet it is failing. since is when the
// current health state began.
func recurringOutage(
	ctx detection.Context, e inventory.Entity, unhealthy bool,
	since time.Time,
) []detection.Finding {
	starts := outageTimes(ctx, episodeKey)
	ends := outageTimes(ctx, episodeEndKey)
	switch {
	case unhealthy && !ctx.Ongoing("outage-ongoing"):
		shiftEpisodes(ctx, &starts, &ends, since)
	case !unhealthy && ctx.Ongoing("outage-ongoing") && ends[0].IsZero():
		ends[0] = since
		ctx.Track(episodeEndKey(0), since)
	}
	if unhealthy {
		ctx.Onset("outage-ongoing", ctx.Now)
	}
	oldest := starts[outageEpisodes-1]
	if oldest.IsZero() || ctx.Now.Sub(oldest) > outageWindow {
		return nil
	}
	ctx.RecheckAfter(outageWindow - ctx.Now.Sub(oldest))
	reason, what := serviceReason(e.ID)
	// Outages that come within minutes of each other are a restart
	// loop; spread over a quarter of an hour, or each too short to be
	// more than a lost probe or two, they are a warning.
	severity := detection.Warning
	if starts[0].Sub(oldest) <= outageRapid &&
		allLasted(ctx.Now, starts, ends) {
		severity = detection.Critical
	}
	return []detection.Finding{{
		Reason: reason, Severity: severity, Since: oldest,
		Summary: what + " has been down " + strconv.Itoa(outageEpisodes) +
			" times in the last " +
			strconv.Itoa(int(outageWindow.Minutes())) +
			" min",
		Evidence: errorEvidence(e),
	}}
}
