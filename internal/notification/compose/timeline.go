package compose

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// timeline renders the last n lines, merging events with the same text in
// the same minute ("3× Pod has not been ready ...") so a burst reads as
// one line. Times are UTC, "15:04"; when the lines span more than one
// day they carry the date too ("2 Jan 15:04"), so a long incident's
// lines are not ambiguous, and the same minute on another day never
// merges.
func timeline(p incident.Incident, n int) []string {
	type line struct {
		minute, text string
		subjects     []string
	}
	var lines []line
	for _, e := range p.Timeline {
		minute := e.At.UTC().Format("2006-01-02 15:04")
		text, subject := splitSubject(e.Text)
		last := len(lines) - 1
		if last >= 0 && lines[last].minute == minute &&
			lines[last].text == text && subject != "" {
			lines[last].subjects = append(lines[last].subjects, subject)
			continue
		}
		lines = append(lines, line{minute: minute, text: text,
			subjects: nonEmpty(subject)})
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	label := clockLabel
	if len(lines) > 0 &&
		lines[0].minute[:10] != lines[len(lines)-1].minute[:10] {
		label = datedLabel
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		text := l.text
		switch len(l.subjects) {
		case 0:
		case 1:
			text += " (" + l.subjects[0] + ")"
		default:
			text = fmt.Sprintf("%s — %d× (%s)", text, len(l.subjects),
				strings.Join(limit(l.subjects, 3), ", ")+
					more(len(l.subjects), 3))
		}
		out = append(out, label(l.minute)+"  "+text)
	}
	return out
}

// clockLabel and datedLabel turn a "2006-01-02 15:04" key into the
// line's time: "15:04", or "2 Jan 15:04" for a timeline over several
// days.
func clockLabel(minute string) string { return minute[11:] }

func datedLabel(minute string) string {
	at, err := time.Parse("2006-01-02 15:04", minute)
	if err != nil {
		return minute
	}
	return at.Format("2 Jan 15:04")
}

// splitSubject separates "text (subject)" as written by the incident
// timeline.
func splitSubject(text string) (string, string) {
	if !strings.HasSuffix(text, ")") {
		return text, ""
	}
	i := strings.LastIndex(text, " (")
	if i < 0 {
		return text, ""
	}
	return text[:i], text[i+2 : len(text)-1]
}
