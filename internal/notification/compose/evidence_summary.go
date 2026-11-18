package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// evidenceSummary is what a Pro message carries for providers that lay
// out the evidence themselves: the key facts, the log quote, the fix
// steps and the incident's shape (addShape). nil without a report: a
// mail or card provider lays out a message with evidence in its
// evidence form, which a shape alone must not switch it to.
func evidenceSummary(
	d incident.Decision, steps []notification.Step,
) *notification.Evidence {
	if len(d.Facts.Report) == 0 {
		return nil
	}
	out := &notification.Evidence{Checks: checksRead(d.Facts.Report),
		Quote: quoteOf(d.Facts.Report)}
	addShape(out, d.Facts)
	if c := d.Facts.Correlation; c != nil {
		out.Confirmed = c.Confirmed != "" &&
			!rootUnseen(d.Incident, d.Facts.Report)
		if c.Confirmed != "" {
			out.Facts = append(out.Facts, plainQuotes(endSentence(
				upperFirst(confirmedText(c.Confirmed)))))
		}
	}
	for _, f := range d.Facts.Evidence {
		out.Facts = append(out.Facts, factLine(f))
	}
	if c := d.Facts.Correlation; c != nil {
		for _, f := range c.Facts {
			out.Facts = append(out.Facts, f.Label+": "+f.Value)
		}
	}
	out.Facts = out.Facts[:min(len(out.Facts), notification.MaxEvidenceFacts)]
	out.Checked = checkedEvidence(d)
	for _, s := range steps {
		if s.Mutating {
			out.Fix = append(out.Fix, notification.EvidenceStep{
				Text: s.Text, Command: s.Command, Mutating: true})
		}
	}
	return out
}

// addShape adds how the incident is shaped: what ties its members
// together, what its failing replicas share and whether users are
// affected, each when known.
func addShape(out *notification.Evidence, f incident.Facts) {
	if f.Grouping != nil {
		out.Grouped = f.Grouping.Text
	}
	if f.Split != nil {
		out.Replicas = append([]string(nil), f.Split.Lines...)
	}
	if f.Users != nil {
		out.Users = f.Users.Text
	}
}

func factLine(f incident.Fact) string {
	if f.Subject != "" {
		return f.Subject + ": " + f.Text
	}
	return f.Text
}

// checksRead counts the checks that ran and read an answer, each once.
func checksRead(report []incident.Section) int {
	seen := map[string]bool{}
	for _, s := range report {
		if s.Status == incident.SectionOK || s.Status == incident.SectionAbsent {
			seen[s.Check] = true
		}
	}
	return len(seen)
}

// quoteOf is the last lines of the first log section that carries the
// signature, ending at its last matched line.
func quoteOf(report []incident.Section) *notification.Quote {
	for _, s := range report {
		if s.Check != incident.CheckLogs || len(s.Matched) == 0 {
			continue
		}
		end := s.Matched[len(s.Matched)-1] + 1
		start := max(0, end-notification.MaxQuoteLines)
		if end > len(s.Lines) {
			continue
		}
		_, container, _ := strings.Cut(s.Subject, "/")
		return &notification.Quote{Container: container, Run: s.Run,
			Lines: append([]string(nil), s.Lines[start:end]...)}
	}
	return nil
}
