package detectors

import (
	"strconv"

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
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow)) {
		if !note.Warning || knownEvent(note.Reason) {
			continue
		}
		ctx.RecheckAfter(note.At.Add(EventWindow).Sub(ctx.Now))
		group, seen := byReason[note.Reason]
		if !seen {
			group = &eventGroup{}
			byReason[note.Reason] = group
			order = append(order, note.Reason)
		}
		group.add(note, detection.Info)
	}
	var out []detection.Finding
	for _, reason := range order {
		group := byReason[reason]
		if group.count < unusualEventMin {
			continue
		}
		out = append(out, detection.Finding{
			Reason:   reasons.UnusualEvent(reason),
			Severity: detection.Info, Mode: detection.ModeUnusualEvent,
			Since: group.newest.At,
			Summary: "Kubernetes reported " + reason + " " +
				strconv.Itoa(group.count) + " times in the last " +
				"quarter hour",
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
