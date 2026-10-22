package pipeline

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory"
)

// applyDowntime applies the changes between saved fingerprints and the
// model and returns the touched entities.
func (e *Engine) applyDowntime(
	saved map[string]string,
) []inventory.EntityID {
	return e.applyChanges(e.downtimeChanges(saved))
}

// downtimeChanges are the net changes between saved fingerprints and the
// model: an object scaled down and back up compares equal and is not one.
func (e *Engine) downtimeChanges(
	saved map[string]string,
) []inventory.Observation {
	if saved == nil || !e.deps.entitled(feature.DowntimeChanges) {
		return nil
	}
	return withoutRestored(e.deps.Model, DowntimeChanges(
		e.deps.Model, saved, e.deps.Synced, e.deps.Clock.Now()))
}

func (e *Engine) applyChanges(
	observations []inventory.Observation,
) []inventory.EntityID {
	if len(observations) == 0 {
		return nil
	}
	klog.InfoS("pipeline: changes made while kwatch was down",
		"component", "pipeline", "count", len(observations))
	return e.apply(observations)
}

// pauseLearningForGap stops baseline learning after a gap until the
// sync, when reconcileDowntime starts the timed pause: the startup storm
// after a gap (every pod restarting at once) must not become normal.
func (e *Engine) pauseLearningForGap() {
	if e.deps.Gap.open() {
		e.storage.history.sampler.pauseUntil(farFuture)
	}
}

// farFuture holds a pause until a real end is known.
var farFuture = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// reportGap sends the gap report once the sources synced: the net
// changes and the crashes inside the gap. Still-broken objects are
// announced as incidents after restoreGrace, not listed here. Baseline
// learning resumes after the same grace.
func (e *Engine) reportGap(changes []inventory.Observation) {
	gap := e.deps.Gap
	if !gap.open() {
		return
	}
	now := e.deps.Clock.Now()
	e.storage.history.sampler.pauseUntil(now.Add(restoreGrace))
	if e.deps.Notify == nil || !e.deps.entitled(feature.GapReport) {
		return
	}
	crashes, rebooted := gapCrashes(e.deps.Model, gap)
	klog.InfoS("pipeline: gap report", "component", "pipeline",
		"changes", len(changes), "crashes", len(crashes),
		"rebootedNodes", rebooted)
	e.deps.Notify(gapReport(gap, changes, crashes, rebooted))
}
