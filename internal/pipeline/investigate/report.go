package investigate

import (
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// Report bounds. The dashboard shows the report whole, so these are
// safety caps, not layout choices: about 64 KB per incident and 500
// lines per log section.
const (
	maxReportBytes      = 64 << 10
	maxSections         = 40
	maxSectionLines     = 500
	maxSectionFacts     = 40
	maxReportText       = 1000
	maxCorrelationFacts = 8
)

// boundReport redacts every section again and keeps it within the
// report caps. Sections past the byte budget are kept as their status
// alone, so the dashboard still shows which checks ran.
func boundReport(report []incident.Section) []incident.Section {
	budget := maxReportBytes
	out := make([]incident.Section, 0, min(len(report), maxSections))
	for _, s := range report {
		if len(out) == maxSections {
			break
		}
		bounded := incident.Section{Check: s.Check, Title: s.Title,
			Status: s.Status, Reason: s.Reason,
			Subject: clipReport(s.Subject), Run: s.Run, At: s.At}
		for _, f := range s.Facts {
			if len(bounded.Facts) == maxSectionFacts || budget <= 0 {
				break
			}
			f = incident.SectionFact{Label: clipReport(f.Label),
				Value: clipReport(redact.Credentials(f.Value))}
			budget -= len(f.Label) + len(f.Value)
			bounded.Facts = append(bounded.Facts, f)
		}
		bounded.Lines, bounded.Matched, budget = boundLines(s, budget)
		out = append(out, bounded)
	}
	return out
}

// boundLines keeps the last lines of s that fit, and the matched
// indexes that still point at a kept line.
func boundLines(s incident.Section, budget int) ([]string, []int, int) {
	lines := s.Lines
	drop := max(0, len(lines)-maxSectionLines)
	lines = lines[drop:]
	var out []string
	for _, line := range lines {
		line = clipReport(redact.Credentials(line))
		if budget-len(line) < 0 {
			break
		}
		budget -= len(line)
		out = append(out, line)
	}
	var matched []int
	for _, i := range s.Matched {
		if j := i - drop; j >= 0 && j < len(out) {
			matched = append(matched, j)
		}
	}
	return out, matched, budget
}

// boundCorrelation redacts and caps the correlation.
func boundCorrelation(c *incident.Correlation) *incident.Correlation {
	if c == nil {
		return nil
	}
	out := &incident.Correlation{
		Confirmed: clipQuoted(redact.Credentials(c.Confirmed))}
	for _, f := range c.Facts {
		if len(out.Facts) == maxCorrelationFacts {
			break
		}
		out.Facts = append(out.Facts, incident.SectionFact{Label: f.Label,
			Value: clipReport(redact.Credentials(f.Value))})
	}
	return out
}

// clipQuoted is clip, closing a quotation the cut left open: a proof
// that quotes a log line ("after logging \"panic: …") must not leave it
// open. The closing quote takes the place of the last byte kept, so the
// result is never longer than clip's. Escaped quotes (\") inside the
// quoted line are not counted.
func clipQuoted(text string) string {
	if len(text) <= maxEvidenceText {
		return text
	}
	cut := maxEvidenceText - 1
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	head := text[:cut]
	if openQuotes(head)%2 == 1 {
		return head + "…\""
	}
	return clip(text)
}

// openQuotes counts the double quotes in text not escaped by a
// backslash.
func openQuotes(text string) int {
	n := 0
	for i := range len(text) {
		if text[i] == '"' && (i == 0 || text[i-1] != '\\') {
			n++
		}
	}
	return n
}

// clipReport cuts text to maxReportText bytes on a rune boundary.
func clipReport(text string) string {
	const n = maxReportText
	if len(text) <= n {
		return text
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

// section builds one report section.
type section struct {
	incident.Section
}

func newSection(subject string) *section {
	return &section{incident.Section{Subject: subject}}
}

// fact adds label and value when value is not empty.
func (s *section) fact(label, value string) {
	if value != "" {
		s.Facts = append(s.Facts,
			incident.SectionFact{Label: label, Value: value})
	}
}

// line adds a line; matched marks it as carrying the signature.
func (s *section) line(text string, matched bool) {
	if matched {
		s.Matched = append(s.Matched, len(s.Lines))
	}
	s.Lines = append(s.Lines, text)
}

func (s *section) done() incident.Section { return s.Section }
