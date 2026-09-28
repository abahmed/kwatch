package scorecard

import (
	"fmt"
	"strings"
)

// Format renders the report for a terminal.
func Format(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "window                 %s\n", r.Window.Round(1e9))
	fmt.Fprintf(&b, "notifications          %d (%.1f/h)\n",
		r.Notifications, r.PerHour)
	fmt.Fprintf(&b, "incidents              %d\n", r.Incidents)
	fmt.Fprintf(&b, "messages per incident  %.2f (p95 %d)\n",
		r.PerIncident, r.PerIncidentP95)
	fmt.Fprintf(&b, "unchanged updates      %d of %d (%.1f%%)\n",
		r.UnchangedUpdates, r.Updates, r.UnchangedUpdatePercent())
	fmt.Fprintf(&b, "re-created incidents   %d\n", r.Recreated)
	fmt.Fprintf(&b, "repeated recoveries    %d\n", r.RepeatedResolves)
	fmt.Fprintf(&b, "grouped messages       %d\n", r.Grouped)
	fmt.Fprintf(&b, "without a cause        %d (%.1f%%)\n",
		r.UnknownCause, r.UnknownCausePercent())
	fmt.Fprintf(&b, "circular causes        %d\n", r.CircularCause)
	b.WriteString("top reasons\n")
	for _, reason := range r.TopReasons {
		fmt.Fprintf(&b, "  %-32s %d\n", reason.Reason, reason.Count)
	}
	return b.String()
}
