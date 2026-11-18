package dashboard

import (
	"slices"
	"strconv"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// SummaryChecked is what kwatch checked for an issue that has no sure
// cause, in plain words: Fine is what it checked and found fine ("its
// node is healthy"), only what it checked for this issue; Also marks
// a cause that is only possible, so the page says "Also ruled out";
// Unchecked is what it could not check ("kwatch cannot read pod logs
// here").
type SummaryChecked struct {
	Fine      []string `json:"fine,omitempty"`
	Also      bool     `json:"also,omitempty"`
	Unchecked []string `json:"unchecked,omitempty"`
}

// logsDenied is what the page says when kwatch may not read pod logs.
const logsDenied = "kwatch cannot read pod logs here"

// checkedOf is what kwatch checked around p, nil when nothing. The
// change history adds a clause only when it was read and nothing near
// p changed in the RecentWindow before it began; kwatch's own lost
// access to pod logs adds what it could not check.
func (c detailContext) checkedOf(
	d *IncidentDetail, p incident.Incident,
) *SummaryChecked {
	r := incident.RuledOutOf(p, p.Evidence, c.quietBefore(d, p))
	if c.logsForbidden && !slices.Contains(r.Unchecked, logsDenied) &&
		(!r.Empty() || (p.Cause == nil && runsCode(p.Root))) {
		r.Unchecked = append(r.Unchecked, logsDenied)
	}
	if r.Empty() {
		return nil
	}
	out := &SummaryChecked{Also: r.Also}
	for _, text := range r.Fine {
		out.Fine = append(out.Fine, redact.Evidence(text))
	}
	out.Unchecked = r.Unchecked
	return out
}

// quietBefore is the clause for an issue without a cause whose change
// history was read and listed no change before it began, else "".
func (c detailContext) quietBefore(
	d *IncidentDetail, p incident.Incident,
) string {
	if !c.changes || c.model == nil || p.Cause != nil ||
		p.Onset().IsZero() {
		return ""
	}
	for _, change := range d.Changes {
		if change.When == "before" {
			return ""
		}
	}
	return "nothing near it was changed in the " +
		strconv.Itoa(int(incident.RecentWindow.Minutes())) +
		" minutes before it started"
}
