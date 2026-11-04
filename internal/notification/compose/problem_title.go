package compose

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// maxProblemTitle bounds a problem title in characters. A title is one
// line of a list; a longer lead is clipped on a word.
const maxProblemTitle = 140

// ProblemTitle is the stable headline of p's problem: the lead of its
// announcement as it would be written now, whatever state the incident
// is in, without the cluster tag and bounded to maxProblemTitle. It is
// never the wording of an update, a recovery or a move, so the
// dashboard names one problem the same way for its whole life. The
// Community writer keeps its own lead: it names what is failing, not
// the cause. names is the declared spelling of custom kinds; nil when
// none is known. Empty for an incident with no member to lead with.
func (w Writer) ProblemTitle(
	p incident.Incident, names map[inventory.Kind]string, now time.Time,
) string {
	if len(p.Members) == 0 {
		return ""
	}
	if p.State != incident.Open && p.State != incident.Flapping {
		// Written as it is while open: a settling incident has the
		// same lead, a resolved one would read as a recovery. A
		// flapping one keeps its own lead ("is not serving: …"), as
		// its alert says it.
		p.State = incident.Open
	}
	d := incident.Decision{Action: incident.Announce, Incident: p}
	d.Facts.KindNames = names
	w.Cluster, w.Dashboard = "", nil
	return clipTitle(strings.TrimSpace(w.write(d, now, nil).Title))
}

// clipTitle cuts title to at most maxProblemTitle characters on a word
// boundary, ending it with an ellipsis.
func clipTitle(title string) string {
	const limit = maxProblemTitle
	if utf8.RuneCountInString(title) <= limit {
		return title
	}
	runes := []rune(title)
	cut := string(runes[:limit-1])
	if i := strings.LastIndexByte(cut, ' '); i > limit/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}
