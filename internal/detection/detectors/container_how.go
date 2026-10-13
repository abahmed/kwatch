package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// How a container failed is told from numbers kwatch already holds
// about its past runs. Nothing here looks ahead.

const (
	// jumpFactor is how many times higher the last reading before a
	// kill must be than the one before it to count as a jump.
	jumpFactor = 1.5
	// jumpMinRise is the least rise, in bytes, that is a jump: a small
	// container doubling is noise.
	jumpMinRise = 64 << 20
	// jumpMaxGap is the longest time between the two readings for the
	// rise to be called sudden.
	jumpMaxGap = 2 * time.Minute
	// minRestartsForSpan is how many restarts make "so many in so long"
	// a rhythm worth saying.
	minRestartsForSpan = 2
)

// lastJump describes a run that rose sharply between its last two
// readings before the kill: "180Mi to 512Mi within 40s".
func lastJump(e inventory.Entity) (string, bool) {
	last, ok := number(e, kube.AttrMemoryPrevLast)
	before, _ := number(e, kube.AttrMemoryPrevBefore)
	gapSecs, _ := number(e, kube.AttrMemoryPrevGapSecs)
	gap := time.Duration(gapSecs * float64(time.Second))
	if !ok || before <= 0 || gap <= 0 || gap > jumpMaxGap ||
		last < jumpFactor*before || last-before < jumpMinRise {
		return "", false
	}
	return quantity(before) + " to " + quantity(last) + " within " +
		format.Duration(gap), true
}

// withHowItFailed adds to each restart-driven finding how long the
// last run lasted before it ended and how long the pod has existed,
// so the restart count reads as a rhythm.
func withHowItFailed(
	ctx detection.Context, e inventory.Entity, found []detection.Finding,
) []detection.Finding {
	restarts, _ := number(e, kube.AttrRestarts)
	if len(found) == 0 || restarts < 1 {
		return found
	}
	ran, hasRun := lastRunLength(e)
	span := podAge(ctx, e)
	for i := range found {
		f := &found[i]
		if hasRun && !quickOOM(f, e) {
			f.Evidence = append(f.Evidence, detection.Evidence{
				Label: detection.EvidenceRanFor,
				Value: format.Duration(max(ran, time.Second))})
		}
		if span > 0 && restarts >= minRestartsForSpan {
			f.Evidence = append(f.Evidence, detection.Evidence{
				Label: detection.EvidenceRestartSpan,
				Value: format.Duration(span)})
		}
	}
	return found
}

// quickOOM reports a kill soon after start, which the memory evidence
// already says in its own words.
func quickOOM(f *detection.Finding, e inventory.Entity) bool {
	if f.Reason != reasons.OOMKilled {
		return false
	}
	_, quick := livedBeforeKill(e)
	return quick
}

// lastRunLength is how long the last terminated run lasted.
func lastRunLength(e inventory.Entity) (time.Duration, bool) {
	started := timestamp(e, kube.AttrLastStarted)
	finished := timestamp(e, kube.AttrLastFinished)
	if started.IsZero() || finished.IsZero() || finished.Before(started) {
		return 0, false
	}
	return finished.Sub(started), true
}

// podAge is how long the container's pod has existed, or zero.
func podAge(ctx detection.Context, e inventory.Entity) time.Duration {
	pod, ok := owningPod(ctx, e)
	if !ok {
		return 0
	}
	created := timestamp(pod, kube.AttrCreated)
	if created.IsZero() || ctx.Now.Before(created) {
		return 0
	}
	return ctx.Now.Sub(created)
}
