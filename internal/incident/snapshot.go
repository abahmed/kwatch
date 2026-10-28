package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Snapshot returns a detached copy safe to hand to writers.
func (inc *Incident) Snapshot() Incident {
	out := *inc
	out.Members = make(map[detection.Key]detection.Finding, len(inc.Members))
	for key, s := range inc.Members {
		out.Members[key] = s
	}
	out.Impact = append([]inventory.EntityID(nil), inc.Impact...)
	out.Merged = append([]inventory.EntityID(nil), inc.Merged...)
	out.Removed = append([]inventory.EntityID(nil), inc.Removed...)
	out.Cycles = append([]time.Time(nil), inc.Cycles...)
	out.Occurrences = append([]time.Time(nil), inc.Occurrences...)
	out.History = append([]Occurrence(nil), inc.History...)
	out.Timeline = append([]Event(nil), inc.Timeline...)
	out.Unverified = append([]string(nil), inc.Unverified...)
	out.Checked = append([]string(nil), inc.Checked...)
	out.Considered = append([]string(nil), inc.Considered...)
	out.Facts = append([]string(nil), inc.Facts...)
	out.Steps = append([]FixStep(nil), inc.Steps...)
	out.AnnouncedRoute = inc.AnnouncedRoute.clone()
	out.Ack = inc.Ack.clone()
	out.Reported, out.ReportedKnown = inc.sent.reported(len(inc.Timeline))
	if inc.Cause != nil {
		cause := *inc.Cause
		out.Cause = &cause
	}
	if inc.FixedBy != nil {
		fix := *inc.FixedBy
		out.FixedBy = &fix
	}
	if inc.Attempt != nil {
		attempt := *inc.Attempt
		out.Attempt = &attempt
	}
	return out
}
