package detection

import (
	"fmt"
	"slices"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/metrics"
)

// incompleteRecheck is how soon an entity whose detector panicked is
// evaluated again: a panic says nothing about the entity, so the next
// pass must not wait for it to change.
const incompleteRecheck = time.Minute

// detectContained runs one detector for one entity. A panic is contained
// at this boundary: that detector yields nothing for this pass and ok is
// false, so the caller knows its findings are unknown and must not read
// the missing ones as "cleared". Every other detector and entity is still
// evaluated.
func detectContained(
	ctx Context, detector Detector, entity inventory.Entity,
) (found []Finding, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			found, ok = nil, false
			metrics.DefaultRegistry().IncPanic("detector")
			klog.ErrorS(nil, "detection: detector panicked",
				"component", "detection", "operation", "detect",
				"detector", detector.Name(), "kind", string(entity.ID.Kind),
				"panic", fmt.Sprint(r))
		}
	}()
	return detect(ctx, detector, entity), true
}

// Keeping returns the findings to track for the entity after this
// evaluation. Without a panic or an unsynced kind that is the fresh
// findings. Otherwise it is the fresh findings of every detector plus the
// previous findings (the tracker's active ones) that an unsure detector
// (one that panicked or needed a kind not synced) raised and no fresh
// finding replaces: its result says nothing about those, so they neither
// clear nor change, while the other detectors keep working.
func (ev Evaluation) Keeping(previous []Finding) []Finding {
	if len(ev.Panicked) == 0 && len(ev.UnsyncedBy) == 0 {
		return ev.Findings
	}
	out := slices.Clone(ev.Findings)
	fresh := make(map[string]bool, len(ev.Findings))
	for _, f := range ev.Findings {
		fresh[f.Reason] = true
	}
	for _, f := range previous {
		if !fresh[f.Reason] && ev.unsure(f) {
			out = append(out, f)
		}
	}
	return out
}

// unsure reports whether the detector that raised f could not say this
// time whether it still holds.
func (ev Evaluation) unsure(f Finding) bool {
	return slices.Contains(ev.Panicked, f.Detector) ||
		slices.Contains(ev.UnsyncedBy, f.Detector)
}
