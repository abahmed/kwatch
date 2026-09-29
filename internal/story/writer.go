package story

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

const (
	maxEvidenceLines = 3
	maxTimelineLines = 6
	maxImpactNames   = 5
)

// Write composes the message for one decision.
func Write(d problem.Decision, now time.Time) Message {
	p := d.Problem
	msg := Message{Key: p.ID, Revision: p.Revision}
	members := sortedMembers(p)
	if d.Action == problem.Resolve {
		msg.Status = StatusResolved
		msg.Title = "Resolved: " + lowerFirst(headline(p, members)) +
			" — lasted " +
			format.Duration(p.Resolved.Sub(p.Opened))
		msg.Lines = []string{"Everything this problem affected is " +
			"healthy again."}
		msg.Timeline = timeline(p, maxTimelineLines)
		return msg
	}
	msg.Status = status(p)
	msg.Title = upperFirst(headline(p, members))
	msg.Lines = append(msg.Lines, whatLines(p, members, now)...)
	msg.Lines = append(msg.Lines, whyLines(p, members)...)
	if line := impactLine(p); line != "" {
		msg.Lines = append(msg.Lines, line)
	}
	msg.Lines = append(msg.Lines, evidenceLines(members)...)
	msg.Timeline = timeline(p, maxTimelineLines)
	msg.Output = d.Output
	msg.Steps = nextSteps(p, members)
	msg.Confidence = confidence(p.Cause)
	return msg
}

func status(p problem.Problem) Status {
	switch {
	case p.State == problem.Flapping:
		return StatusFlapping
	case p.Tier == problem.Page:
		return StatusCritical
	default:
		return StatusWarning
	}
}

// headline names the root and, when proven, the cause in reader terms.
func headline(p problem.Problem, members []signal.Signal) string {
	switch {
	case p.State == problem.Flapping:
		return describe(p.Root) + " keeps failing and recovering"
	case p.Cause != nil && p.Cause.Change != nil:
		return describe(affectedWorkload(p)) + " is failing after " +
			p.Cause.Summary + affectedSuffix(p)
	}
	subject := describe(p.Root)
	if own := rootSignal(p, members); own != nil {
		subject = withSubject(p.Root, own.Summary)
	} else if p.Cause != nil {
		subject = p.Cause.Summary
	} else if len(members) > 0 {
		subject += ": " + lowerFirst(members[0].Summary)
	}
	return subject + affectedSuffix(p)
}

func headlineStatesCause(p problem.Problem, members []signal.Signal) bool {
	if p.Cause == nil || p.Cause.Change != nil {
		return false
	}
	return rootSignal(p, members) != nil || p.Cause.Root == p.Root
}

// withSubject joins an entity and a detector summary, dropping the
// summary's leading kind word: "Node is low on memory" about node n1
// becomes "node n1 is low on memory".
func withSubject(id knowledge.EntityID, summary string) string {
	kind := upperFirst(string(id.Kind)) + " "
	if strings.HasPrefix(summary, kind) {
		return describe(id) + " " + summary[len(kind):]
	}
	return describe(id) + ": " + lowerFirst(summary)
}

// rootSignal is the root's own most severe condition, if it has one.
// Symptoms (a workload below its replicas) never lead the message.
func rootSignal(p problem.Problem, members []signal.Signal) *signal.Signal {
	for i := range members {
		if members[i].Entity == p.Root && !members[i].Symptom {
			return &members[i]
		}
	}
	return nil
}

// affectedWorkloads names the workloads a problem affects beyond its root.
func affectedWorkloads(p problem.Problem) []string {
	var names []string
	for _, id := range p.Impact {
		if isWorkload(id.Kind) && id != p.Root {
			names = append(names, describe(id))
		}
	}
	return names
}

// affectedSuffix counts the workloads a problem affects beyond its root.
func affectedSuffix(p problem.Problem) string {
	switch workloads := len(affectedWorkloads(p)); workloads {
	case 0:
		return ""
	case 1:
		return " — 1 workload affected"
	default:
		return fmt.Sprintf(" — %d workloads affected", workloads)
	}
}

func whatLines(
	p problem.Problem, members []signal.Signal, now time.Time,
) []string {
	if len(members) == 0 {
		return nil
	}
	if own := rootSignal(p, members); own != nil {
		line := "Started " + format.Duration(now.Sub(own.Since)) + " ago."
		if names := affectedWorkloads(p); len(names) > 0 {
			line += " Affected: " +
				strings.Join(limit(names, maxImpactNames), ", ") +
				more(len(names), maxImpactNames) + "."
		}
		return []string{line}
	}
	first := members[0]
	line := first.Summary + " (for " +
		format.Duration(now.Sub(first.Since)) + ")."
	switch related := len(members) - 1; {
	case related == 1:
		line += " 1 related failure is grouped here."
	case related > 1:
		line += fmt.Sprintf(" %d related failures are grouped here.",
			related)
	}
	return []string{line}
}

func whyLines(p problem.Problem, members []signal.Signal) []string {
	if p.Cause == nil && rootSignal(p, members) != nil {
		return nil
	}
	if p.Cause == nil {
		return []string{"No upstream cause was found: the node, recent " +
			"changes and dependencies look healthy, so the application " +
			"itself is the most likely source. Its error output is below."}
	}
	var reasons []string
	for _, point := range p.Cause.Points {
		if point.Supports {
			reasons = append(reasons, point.Text)
		}
	}
	// When the headline already states the cause, repeating it adds
	// nothing; only the evidence is new.
	if headlineStatesCause(p, members) {
		if len(reasons) == 0 {
			return nil
		}
		return []string{"How we know: " +
			strings.Join(limit(reasons, 3), "; ") + "."}
	}
	if p.Cause.Change != nil && len(reasons) > 0 {
		// The headline already names the change; say why it is blamed.
		return []string{"Why this change: " +
			strings.Join(limit(reasons, 3), "; ") + "."}
	}
	line := "Why: " + p.Cause.Summary + "."
	if len(reasons) > 0 {
		line += " Evidence: " + strings.Join(limit(reasons, 3), "; ") + "."
	}
	return []string{line}
}

func impactLine(p problem.Problem) string {
	var names []string
	for _, id := range p.Impact {
		if id.Kind == "service" || id.Kind == "ingress" {
			names = append(names, describe(id))
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "Impact: " + strings.Join(limit(names, maxImpactNames), ", ") +
		more(len(names), maxImpactNames) + " cannot serve traffic."
}

func evidenceLines(members []signal.Signal) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range members {
		for _, e := range s.Evidence {
			line := e.Label + ": " + e.Value
			if e.Value == "" || seen[line] || hiddenEvidence[e.Label] {
				continue
			}
			seen[line] = true
			out = append(out, line)
		}
	}
	return limit(out, maxEvidenceLines)
}

// timeline renders the last n lines, merging events with the same text in
// the same minute ("3× Pod has not been ready ...") so a burst reads as
// one line.
// hiddenEvidence labels are already expressed elsewhere in the message.
var hiddenEvidence = map[string]bool{"image": true, "missing": true}

func timeline(p problem.Problem, n int) []string {
	type line struct {
		minute, text string
		subjects     []string
	}
	var lines []line
	for _, e := range p.Timeline {
		minute := e.At.UTC().Format("15:04")
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
		out = append(out, l.minute+"  "+text)
	}
	return out
}

// splitSubject separates "text (subject)" as written by the problem
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

func nonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func confidence(cause *reason.Hypothesis) string {
	switch {
	case cause == nil:
		return ""
	case cause.Score >= reason.High:
		return "high"
	case cause.Score >= reason.Likely:
		return "likely"
	default:
		return "possible"
	}
}

func sortedMembers(p problem.Problem) []signal.Signal {
	out := make([]signal.Signal, 0, len(p.Members))
	for _, s := range p.Members {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity > out[j].Severity
		}
		return out[i].Since.Before(out[j].Since)
	})
	return out
}

// affectedWorkload is the workload a change broke: the first workload in
// the cause chain, or the root.
func affectedWorkload(p problem.Problem) knowledge.EntityID {
	if p.Cause != nil {
		for _, id := range p.Cause.Chain {
			if isWorkload(id.Kind) {
				return id
			}
		}
	}
	return p.Root
}

func describe(id knowledge.EntityID) string {
	name := id.Name
	if id.Namespace != "" {
		name = id.Namespace + "/" + id.Name
	}
	return string(id.Kind) + " " + name
}

func limit(values []string, n int) []string {
	if len(values) > n {
		return values[:n]
	}
	return values
}

func more(total, shown int) string {
	if total <= shown {
		return ""
	}
	return fmt.Sprintf(" and %d more", total-shown)
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}
