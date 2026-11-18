package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// checkedEvidence says, for the evidence of a message about an issue
// without a sure cause, what kwatch checked for this issue and found
// fine, at most notification.MaxEvidenceChecked items, and what it
// could not check. With a cause that is only possible the list reads
// "Also ruled out: …". Nothing checked says nothing: no empty heading.
func checkedEvidence(d incident.Decision) []string {
	r := incident.RuledOutOf(d.Incident, d.Facts.Report, quietHistory(d))
	var out []string
	if fine := r.Fine; len(fine) > 0 {
		fine = fine[:min(len(fine), notification.MaxEvidenceChecked)]
		if r.Also {
			out = append(out, "Also ruled out: "+joinWords(fine)+".")
		} else {
			out = append(out, "kwatch checked: "+
				strings.Join(fine, "; ")+".")
		}
	}
	for _, text := range r.Unchecked {
		if !strings.HasPrefix(text, "kwatch ") {
			text = upperFirst(text)
		}
		out = append(out, endSentence(text))
	}
	return out
}

// quietHistory is the clause for a namespace where the change history
// was read and nothing was changed in the last RecentWindow, or "": a
// message with a cause, a listed change or a health window has changes
// to say instead.
func quietHistory(d incident.Decision) string {
	ns := d.Incident.Root.Namespace
	if !d.Facts.ChangesRead || ns == "" || d.Incident.Cause != nil ||
		len(d.Facts.Changes) > 0 || d.Facts.Window != nil {
		return ""
	}
	return "nothing in " + ns + " was changed in the last " +
		strconv.Itoa(int(incident.RecentWindow.Minutes())) + " minutes"
}
