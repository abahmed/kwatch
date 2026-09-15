package event

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// MaxPodEventsInMessage bounds Kubernetes event evidence included in an
// alert. Keeping this formatter with the event model avoids provider or
// monitor packages growing their own event rendering rules.
const MaxPodEventsInMessage = 40

// FormatPodEvents renders the newest Kubernetes events in chronological
// order. Older events are explicitly counted instead of silently discarded.
func FormatPodEvents(events *[]corev1.Event) string {
	if events == nil {
		return ""
	}
	sorted := make([]corev1.Event, len(*events))
	copy(sorted, *events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return podEventTime(sorted[i]).Before(podEventTime(sorted[j]))
	})
	omitted := 0
	if len(sorted) > MaxPodEventsInMessage {
		omitted = len(sorted) - MaxPodEventsInMessage
		sorted = sorted[len(sorted)-MaxPodEventsInMessage:]
	}
	var builder strings.Builder
	if omitted > 0 {
		fmt.Fprintf(
			&builder,
			"... %d earlier event(s) omitted\n",
			omitted,
		)
	}
	for _, item := range sorted {
		timestamp := podEventTime(item)
		if timestamp.IsZero() {
			fmt.Fprintf(&builder, "%s %s\n", item.Reason, item.Message)
			continue
		}
		fmt.Fprintf(
			&builder,
			"%s  %s  %s\n",
			timestamp.UTC().Format("Jan 02 15:04:05"),
			item.Reason,
			item.Message,
		)
	}
	return strings.TrimSpace(builder.String())
}

func podEventTime(item corev1.Event) time.Time {
	for _, timestamp := range []time.Time{
		item.LastTimestamp.Time,
		item.EventTime.Time,
		item.FirstTimestamp.Time,
		item.CreationTimestamp.Time,
	} {
		if !timestamp.IsZero() {
			return timestamp
		}
	}
	return time.Time{}
}
