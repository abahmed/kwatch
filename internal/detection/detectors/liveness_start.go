package detectors

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	// recentStarts is how many of the workload's last starts say how
	// long a start takes.
	recentStarts = 5
	// minStarts is how few starts still say it: one slow pod is not a
	// pattern.
	minStarts = 2
	// killSlack is how much longer than the liveness budget a run may
	// last and still have been ended by that budget: the kubelet's
	// probe timeout and the time to stop the process.
	killSlack = 15 * time.Second
)

// withStartFacts adds to a liveness kill what it says about the start:
// what liveness allows and how long the workload's starts took, so the
// message can say the kill came before the start could finish. Only a
// container that was killed before it was ready gets them; one that
// ran long enough to start and then failed is not starting.
func withStartFacts(
	ctx detection.Context, e inventory.Entity, found detection.Finding,
) detection.Finding {
	found.Evidence = append(found.Evidence, startEvidence(ctx, e)...)
	found.Evidence = append(found.Evidence, sameCheckEvidence(e)...)
	return found
}

// sameCheckEvidence says that the liveness probe runs the readiness
// probe's check, so what fails readiness fails liveness too.
func sameCheckEvidence(e inventory.Entity) []detection.Evidence {
	if !flag(e, kube.AttrLivenessSameCheck) {
		return nil
	}
	return []detection.Evidence{{
		Label: detection.EvidenceLivenessSameCheck, Value: "true"}}
}

func startEvidence(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	grace, parts, ok := livenessGrace(e)
	run, known := lastRun(e)
	if !ok || !known {
		return nil
	}
	gives := detection.Evidence{Label: detection.EvidenceLivenessGives,
		Value: shortSeconds(grace) + " (" + parts + ")"}
	usual, count, seen := usualStart(ctx, e)
	if seen {
		if grace >= usual || run >= usual {
			return nil
		}
		return []detection.Evidence{gives,
			{Label: detection.EvidenceUsualStart, Value: shortSeconds(usual)},
			{Label: detection.EvidenceStartSamples,
				Value: strconv.Itoa(count)}}
	}
	if !strings.Contains(text(e, kube.AttrProbes), "readiness") ||
		run > grace+killSlack {
		return nil
	}
	return []detection.Evidence{gives, {
		Label: detection.EvidenceKilledBeforeReady, Value: "true"}}
}

// livenessGrace is how long liveness lets a container run before the
// kill (delay plus every allowed failure) and how it adds up.
func livenessGrace(e inventory.Entity) (time.Duration, string, bool) {
	delay, ok1 := number(e, kube.AttrLivenessDelay)
	period, ok2 := number(e, kube.AttrLivenessPeriod)
	failures, ok3 := number(e, kube.AttrLivenessFailures)
	if !ok1 || !ok2 || !ok3 {
		return 0, "", false
	}
	total := time.Duration(delay+period*failures) * time.Second
	parts := strconv.Itoa(int(delay)) + "s delay + " +
		strconv.Itoa(int(failures)) + " × " + strconv.Itoa(int(period)) + "s"
	return total, parts, true
}

// lastRun is how long the container's last terminated run lasted.
func lastRun(e inventory.Entity) (time.Duration, bool) {
	started := timestamp(e, kube.AttrLastStarted)
	finished := timestamp(e, kube.AttrLastFinished)
	if started.IsZero() || finished.Before(started) {
		return 0, false
	}
	return finished.Sub(started), true
}

// usualStart is the middle of the workload's last starts, in seconds
// to ready. They come from its own baseline (pods of healthy replicas
// and earlier revisions that became ready), else from the ready pods
// running now. It reports how many starts that is.
func usualStart(
	ctx detection.Context, e inventory.Entity,
) (time.Duration, int, bool) {
	workload, ok := workloadOfContainer(ctx, e)
	if !ok {
		return 0, 0, false
	}
	starts := learnedStarts(ctx, workload)
	if len(starts) < minStarts {
		starts = runningStarts(ctx, workload)
	}
	if len(starts) < minStarts {
		return 0, 0, false
	}
	recent := slices.Clone(starts[max(0, len(starts)-recentStarts):])
	slices.Sort(recent)
	middle := recent[len(recent)/2]
	return time.Duration(middle * float64(time.Second)), len(recent), true
}

// learnedStarts are the start times the workload's baseline holds,
// oldest first.
func learnedStarts(
	ctx detection.Context, workload inventory.EntityID,
) []float64 {
	b := baselinesOf(ctx)
	if b == nil {
		return nil
	}
	stat, ok := b.Stat(workload, inventory.MetricStartSeconds)
	if !ok {
		return nil
	}
	return stat.Recent
}

// maxStartPods bounds the pods read for their start time.
const maxStartPods = 64

// runningStarts are the start times of the workload's pods that are
// ready now, oldest ready first, so the last ones are the newest.
func runningStarts(
	ctx detection.Context, workload inventory.EntityID,
) []float64 {
	type start struct {
		ready   time.Time
		seconds float64
	}
	var found []start
	pods := kube.OwnedPods(ctx.Model, workload)
	for _, id := range pods[:min(len(pods), maxStartPods)] {
		pod, ok := ctx.Model.Entity(id)
		if !ok || !flag(pod, kube.AttrReady) {
			continue
		}
		began := timestamp(pod, kube.AttrContainersStarted)
		if began.IsZero() {
			began = timestamp(pod, kube.AttrStartTime)
		}
		ready := timestamp(pod, kube.AttrReadySince)
		if began.IsZero() || ready.IsZero() || ready.Before(began) {
			continue
		}
		found = append(found, start{ready, ready.Sub(began).Seconds()})
	}
	slices.SortFunc(found, func(a, b start) int {
		return a.ready.Compare(b.ready)
	})
	out := make([]float64, 0, len(found))
	for _, f := range found {
		out = append(out, f.seconds)
	}
	return out
}

// shortSeconds writes d the way a probe is configured: "75s".
func shortSeconds(d time.Duration) string {
	return strconv.Itoa(int(d.Round(time.Second)/time.Second)) + "s"
}
