package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// fleetSilentGrace is how long every node going silent at once is held
// before it is reported. All kubelets stopping together points at the
// path they report through, which usually returns within minutes.
const fleetSilentGrace = 4 * time.Minute

// fleetSilent reports whether two or more nodes exist, every one of them
// reads Ready=Unknown and no pod has been affected: with pods unready
// the outage is real and pages at the usual pace.
func fleetSilent(ctx detection.Context) bool {
	if ctx.Model == nil {
		return false
	}
	ids := ctx.Model.EntitiesIn(kube.KindNode, "")
	if len(ids) < 2 {
		return false
	}
	for _, id := range ids {
		n, ok := ctx.Model.Entity(id)
		if !ok {
			return false
		}
		if status, _, _ := condition(n, "Ready"); status != "Unknown" {
			return false
		}
	}
	return !podsAffected(ctx)
}

// podsAffected reports whether a running pod is not ready. A pod that
// finished (a completed Job run, an evicted pod) is never ready again,
// so it says nothing about the outage now.
func podsAffected(ctx detection.Context) bool {
	for _, id := range ctx.Model.Entities(kube.KindPod) {
		pod, ok := ctx.Model.Entity(id)
		if !ok || kube.PodFinished(pod) {
			continue
		}
		if status, _, _ := condition(pod, "Ready"); status == "False" {
			return true
		}
	}
	return false
}

// fleetRecheck is how often a held fleet is looked at again, so pods
// going unready end the hold within a moment.
const fleetRecheck = 15 * time.Second

// silentGrace is the wait before a silent node is reported: the longer
// fleetSilentGrace while the whole fleet is silent and nothing running on
// it has been affected. A pod change does not re-evaluate a node, so the
// hold asks to be looked at again when the usual wait is up and then
// every fleetRecheck.
func silentGrace(
	ctx detection.Context, since time.Time, grace time.Duration,
) time.Duration {
	if grace >= fleetSilentGrace || !fleetSilent(ctx) {
		return grace
	}
	wait := fleetRecheck
	if !since.IsZero() {
		if left := grace - ctx.Now.Sub(since); left > 0 {
			wait = left
		}
	}
	ctx.RecheckAfter(wait)
	return fleetSilentGrace
}
