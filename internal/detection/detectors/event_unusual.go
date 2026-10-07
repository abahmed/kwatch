package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// unusualEventMin is how many times a Warning event kwatch has no
// detector for must repeat within EventWindow before it is a finding.
// One such event is noise; the same one three times is a pattern worth
// a look in the digest.
const unusualEventMin = 3

// shownByState are Warning event reasons whose condition kwatch already
// reads from the object itself: a container in BackOff, a probe that
// fails, a pod that cannot be scheduled. Reporting the event too would
// report the failure twice.
var shownByState = map[string]bool{
	"BackOff": true, "Unhealthy": true, "FailedScheduling": true,
	"Evicted": true, "Failed": true, "Killing": true, "Preempting": true,
	"Preempted": true, "ErrImagePull": true, "ImagePullBackOff": true,
	"InspectFailed": true, "NodeNotReady": true, "NodeNotSchedulable": true,
	"BackoffLimitExceeded": true, "DeadlineExceeded": true,
	"TooManyMissedTimes": true, "FailedComputeMetricsReplicas": true,
	"FailedGetResourceMetric": true, "FailedGetScale": true,
	"FailedToUpdateEndpointSlices": true, "FailedToUpdateEndpoint": true,
	"OOMKilling": true, "FailedCallingWebhook": true,
	"FailedValidation": true, "NoPods": true,
	"CalculateExpectedPodCountFailed": true,
}

// unusualEvents turns Warning events kwatch knows nothing about into one
// informational finding per event reason, once they repeat. The event
// text is quoted, never interpreted: the finding says what Kubernetes
// said and how often, so a new failure type is seen before kwatch has a
// detector for it.
func unusualEvents(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var order []string
	byReason := map[string]*eventGroup{}
	// recurred marks a reason seen again in a later quarter hour than
	// its first sighting. It counts as repeating even when each sighting
	// came as a fresh Event with a count of one (the model keeps one
	// note per source and reason, so such counts do not add up). A
	// failure that recurs after its first finding cleared is reported
	// again this way.
	recurred := map[string]bool{}
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow)) {
		if !note.Warning || knownEvent(note.Reason) ||
			quotedByService(e, note.Reason) {
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
		if note.At.Sub(note.FirstSeen) > EventWindow {
			recurred[note.Reason] = true
		}
	}
	var out []detection.Finding
	for _, reason := range order {
		group := byReason[reason]
		if group.count < unusualEventMin && !recurred[reason] {
			continue
		}
		out = append(out, detection.Finding{
			Reason:   reasons.UnusualEvent(reason),
			Severity: detection.Info, Mode: detection.ModeUnusualEvent,
			Since:   group.newest.At,
			Summary: unusualSummary(reason, group.count),
			Evidence: []detection.Evidence{
				{Label: "event", Value: group.newest.Message},
				{Label: "occurrences", Value: strconv.Itoa(group.count)},
			},
		})
	}
	return out
}

// knownEvent reports whether a Warning event reason is already detected,
// either from the event itself or from the object's state.
func knownEvent(reason string) bool {
	_, detected := eventReasons[reason]
	return detected || shownByState[reason]
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
