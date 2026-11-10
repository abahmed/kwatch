package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// oldProblemSource is what the incident reader may say about the old
// problems it opened no issue for (incident/old_problem.go).
// *incident.Manager says it; a reader without it says nothing.
type oldProblemSource interface {
	OldProblems() []detection.Finding
}

// oldProblemDaysSource is the reader that also says how many days make
// a problem old (the configured oldProblemDays).
type oldProblemDaysSource interface {
	OldProblemDays() int
}

// OldProblems are the low problems that were already old when kwatch
// first saw them, so they opened no issue: the overview says "N old
// problems not shown". Items lists the first maxOldProblems. Days is
// the age past which a problem is old, when the reader says it.
type OldProblems struct {
	Count int          `json:"count"`
	Days  int          `json:"days,omitempty"`
	Items []OldProblem `json:"items,omitempty"`
}

// OldProblem is one of them: the object, what is wrong with it in one
// line, since when and how many whole days ago that was.
type OldProblem struct {
	Kind      string    `json:"kind"`
	Namespace string    `json:"namespace,omitempty"`
	Name      string    `json:"name"`
	Reason    string    `json:"reason"`
	What      string    `json:"what"`
	Since     time.Time `json:"since"`
	AgeDays   int       `json:"ageDays"`
}

// maxOldProblems bounds the old problems the overview lists.
const maxOldProblems = 50

// oldProblems is the count and first entries of the old problems at
// now; nil when the incident reader cannot say.
func (s *Server) oldProblems(now time.Time) *OldProblems {
	reader := s.sources.incidents.Load()
	if reader == nil {
		return nil
	}
	source, ok := (*reader).(oldProblemSource)
	if !ok {
		return nil
	}
	out := buildOldProblems(source.OldProblems(), now, s.texts.redact)
	if days, ok := (*reader).(oldProblemDaysSource); ok {
		out.Days = max(days.OldProblemDays(), 0)
	}
	return out
}

// buildOldProblems turns the findings into the overview's entries.
func buildOldProblems(
	findings []detection.Finding, now time.Time, red func(string) string,
) *OldProblems {
	if red == nil {
		red = func(s string) string { return s }
	}
	out := &OldProblems{Count: len(findings)}
	for _, f := range findings[:min(len(findings), maxOldProblems)] {
		out.Items = append(out.Items, OldProblem{
			Kind:      kindDisplay(f.Entity),
			Namespace: f.Entity.Namespace, Name: f.Entity.Name,
			Reason: f.Reason, What: red(f.Summary), Since: f.Since,
			AgeDays: int(now.Sub(f.Since) / (24 * time.Hour))})
	}
	return out
}
