package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// unusualEventMin is how many times a Warning event kwatch has no
// detector for must repeat within EventWindow before it is a finding.
// One such event is noise; the same one three times is a pattern worth
// a look in the digest.
const unusualEventMin = 3

// unusualEvents turns Warning events kwatch knows nothing about into one
// informational finding per event reason, once they repeat. The event
// text is quoted, never interpreted: the finding says what Kubernetes
// said and how often, so a new failure type is seen before kwatch has a
// detector for it.
func unusualEvents(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	custom := flag(e, kube.AttrCustom)
	if _, healthy := readySince(e); custom && healthy {
		// Operators write Warnings about what they process onto their
		// own object. A resource that reports itself healthy is not what
		// is wrong, however many there are.
		return nil
	}
	var order []string
	byReason := map[string]*eventGroup{}
	since := ctx.Now.Add(-EventWindow)
	// recent counts each reason's occurrences inside the window: a note's
	// own count also holds Events of hours ago.
	recent := map[string]int{}
	// recurred marks a reason seen again in a later quarter hour than
	// its first sighting. It counts as repeating even when each sighting
	// came as a fresh Event with a count of one, too few to reach
	// unusualEventMin. A failure that recurs after its first finding
	// cleared is reported again this way. A custom resource needs
	// unusualEventMin in the window: controllers there write one fresh
	// event per item.
	recurred := map[string]bool{}
	for _, note := range ctx.Model.Notes(e.ID, since) {
		if !note.Warning || knownEvent(note.Reason, e.ID.Kind) ||
			quotedByService(e, note.Reason) || recoveredBefore(e, note) {
			continue
		}
		// The window includes a note exactly EventWindow old, so the
		// recheck falls just after it: a finding must clear when its
		// last event ages out, not stay until the next event.
		ctx.RecheckAfter(
			note.At.Add(EventWindow).Sub(ctx.Now) + time.Nanosecond)
		group, seen := byReason[note.Reason]
		if !seen {
			group = &eventGroup{}
			byReason[note.Reason] = group
			order = append(order, note.Reason)
		}
		group.add(note, detection.Info)
		recent[note.Reason] += note.CountSince(since)
		if !custom && note.At.Sub(note.FirstSeen) > EventWindow {
			recurred[note.Reason] = true
		}
	}
	var out []detection.Finding
	for _, reason := range order {
		group := byReason[reason]
		if recent[reason] < unusualEventMin && !recurred[reason] {
			continue
		}
		out = append(out, detection.Finding{
			Reason:   reasons.UnusualEvent(reason),
			Severity: detection.Info, Mode: detection.ModeUnusualEvent,
			Since:   group.newest.At,
			Summary: unusualSummary(reason, recent[reason]),
			Evidence: []detection.Evidence{
				{Label: "event", Value: group.newest.Message},
				{Label: "occurrences", Value: strconv.Itoa(recent[reason])},
			},
		})
	}
	return out
}

// knownEvent reports whether a Warning event reason is already detected,
// either from the event itself or from the state of an object of the
// kind. The same reason on another kind is unknown: a custom resource's
// controller may say "Failed" or "BackOff" about something no state shows.
func knownEvent(reason string, kind inventory.Kind) bool {
	_, detected := detection.EventSeverity(reason)
	return detected || detection.ShownByStateOn(reason, kind)
}

// unusualSummary says what Kubernetes reported. A reason that recurred
// with a count of one or two has no count worth quoting.
func unusualSummary(reason string, count int) string {
	if count < unusualEventMin {
		return "Kubernetes reported " + reason + " again"
	}
	return "Kubernetes reported " + reason + " " + strconv.Itoa(count) +
		" times in the last quarter hour"
}
