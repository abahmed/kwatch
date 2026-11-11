package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// titleProblems writes the problem title of every incident whose
// problem opened or changed in this solve (see incident/problem.go):
// the lead of its announcement, before any message is sent. An
// incident that settles before alerting, and one only a digest lists,
// is then named by what fails rather than by its root's key. A writer
// that panics on one incident costs that title only.
func (a *announcer) titleProblems(now time.Time) {
	for _, p := range a.incidents.TakeUntitled() {
		contain("compose", "problem title", func() {
			short, full := a.problemTitle(p, now)
			a.incidents.SetProblem(p.ID, short, full)
		})
	}
}

// problemTitle is the composer's problem title of p, bounded and uncut.
func (a *announcer) problemTitle(
	p incident.Incident, now time.Time,
) (string, string) {
	return a.messages.ProblemTitles(p, a.kindNames(p), now)
}
