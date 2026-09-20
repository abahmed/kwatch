package message

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

type timelinePoint struct {
	at    time.Time
	label string
}

// buildTimeline keeps only milestones a responder can act on. It deliberately
// does not copy raw Events or logs into the incident message.
func (rb *ReportBuilder) buildTimeline(
	inc *model.Incident,
	ins *insight.Insight,
	action model.IncidentAction,
) string {
	if inc == nil {
		return ""
	}
	points := make([]timelinePoint, 0, 6)
	if !inc.FirstSeen.IsZero() {
		points = append(points, timelinePoint{inc.FirstSeen, "started"})
	}
	if inc.LastContainerState != nil && !inc.LastContainerState.LastTerminatedOn.IsZero() {
		points = append(points, timelinePoint{inc.LastContainerState.LastTerminatedOn, "container terminated"})
	}
	if ins != nil {
		for _, change := range ins.RecentChanges {
			if change.Timestamp.IsZero() {
				continue
			}
			label := fmt.Sprintf("%s %s %s", change.Resource, change.Name, change.Type)
			if change.Namespace != "" {
				label = fmt.Sprintf("%s %s/%s %s", change.Resource, change.Namespace, change.Name, change.Type)
			}
			points = append(points, timelinePoint{change.Timestamp, label})
		}
	}
	if inc.LastSeen.IsZero() {
		return ""
	}
	end := "ongoing"
	if action == model.ActionResolved {
		end = "resolved"
	}
	points = append(points, timelinePoint{inc.LastSeen, end})
	sort.SliceStable(points, func(i, j int) bool { return points[i].at.Before(points[j].at) })
	if len(points) < 2 {
		return ""
	}
	points = collapseRepeats(points)
	if len(points) > 5 {
		points = append(points[:4], points[len(points)-1])
	}
	parts := make([]string, 0, len(points))
	for _, point := range points {
		parts = append(parts, point.at.UTC().Format("15:04")+" "+point.label)
	}
	// The clock is the API server's, not the reader's; say which one, or a
	// 16:56 in the timeline sits under a 7:57 PM post and reads as a lie.
	return strings.Join(parts, " → ") + " (UTC)"
}

// collapseRepeats folds a run of identical labels within one minute into a
// single point with a multiplier. Three "hpa updated" entries at 16:56 told
// the reader nothing that one "hpa updated ×3" does not.
func collapseRepeats(points []timelinePoint) []timelinePoint {
	if len(points) < 2 {
		return points
	}
	out := make([]timelinePoint, 0, len(points))
	run := 1
	for i, point := range points {
		if i > 0 && point.label == points[i-1].label &&
			point.at.Truncate(time.Minute).Equal(
				points[i-1].at.Truncate(time.Minute),
			) {
			run++
			continue
		}
		if run > 1 {
			out[len(out)-1].label += fmt.Sprintf(" ×%d", run)
			run = 1
		}
		out = append(out, point)
	}
	if run > 1 {
		out[len(out)-1].label += fmt.Sprintf(" ×%d", run)
	}
	return out
}
