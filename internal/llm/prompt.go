package llm

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

const (
	modelName      = "kwatch-triage"
	RequestTimeout = 60 * time.Second
	maxLogChars    = 6000
	maxEventChars  = 2000
)

func (c *Client) userPrompt(inc *model.Incident) string {
	logs := c.redactor.scrub(selectRelevant(inc.Logs, maxLogChars))
	events := c.redactor.scrub(tailChars(inc.Events, maxEventChars))
	var b strings.Builder
	fmt.Fprintf(&b, "Reason: %s\n", c.redactor.scrub(inc.Reason))
	fmt.Fprintf(&b, "Workload: %s\nOwnerKind: %s\nNamespace: %s\n",
		c.redactor.scrub(inc.Name), c.redactor.scrub(inc.OwnerKind), inc.Namespace)
	if inc.ContainerName != "" {
		fmt.Fprintf(&b, "Container: %s\n", c.redactor.scrub(inc.ContainerName))
	}
	fmt.Fprintf(&b, "RestartCount: %d\n", inc.RestartCount)
	// CD-1: ground with facts kwatch already has
	fmt.Fprintf(&b, "Occurrences: %d\nAffectedPods: %d\nDurationMin: %.0f\n",
		inc.Count, len(inc.Resources), inc.LastSeen.Sub(inc.FirstSeen).Minutes())
	if inc.PeakResources > 0 {
		fmt.Fprintf(&b, "PeakAffected: %d\n", inc.PeakResources)
	}
	if len(inc.Containers) > 1 {
		names := make([]string, 0, len(inc.Containers))
		for n := range inc.Containers {
			names = append(names, n)
		}
		fmt.Fprintf(&b, "Containers: %s\n", strings.Join(names, ", "))
	}
	// Runbook link (CD-4)
	if inc.Runbook != "" {
		fmt.Fprintf(&b, "Runbook: %s\n", c.redactor.scrub(inc.Runbook))
	}
	if inc.Hint != "" {
		fmt.Fprintf(&b, "Rule-based hint: %s\n", c.redactor.scrub(inc.Hint))
	}
	if events != "" {
		fmt.Fprintf(&b, "\nEvents:\n%s\n", events)
	}
	if logs != "" {
		fmt.Fprintf(&b, "\nLogs:\n%s\n", logs)
	}
	return b.String()
}

func tailChars(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	s = s[len(s)-max:]
	if i := strings.IndexByte(s, '\n'); i >= 0 && i+1 < len(s) {
		s = s[i+1:]
	}
	return s
}

var logSignal = regexp.MustCompile(`(?i)\b(panic|fatal|error|exception|traceback|stack trace|oom|out of memory|killed|exit (code|status)|failed|cannot|unable to|refused|timeout|denied)\b`)

func selectRelevant(logs string, max int) string {
	if max <= 0 || len(logs) <= max {
		return logs
	}
	lines := strings.Split(logs, "\n")
	tailBudget := max * 6 / 10

	used, tailStart := 0, len(lines)
	for i := len(lines) - 1; i >= 0 && used < tailBudget; i-- {
		used += len(lines[i]) + 1
		tailStart = i
	}
	headBudget := max - used
	var head []string
	for i := 0; i < tailStart && headBudget > 0; i++ {
		if logSignal.MatchString(lines[i]) {
			head = append(head, lines[i])
			headBudget -= len(lines[i]) + 1
		}
	}

	var b strings.Builder
	if len(head) > 0 {
		b.WriteString(strings.Join(head, "\n"))
		b.WriteString("\n... (older lines omitted) ...\n")
	}
	b.WriteString(strings.Join(lines[tailStart:], "\n"))
	out := b.String()

	if strings.TrimSpace(out) == "" {
		out = tailChars(logs, max)
	}
	if len(out) > max {
		out = tailChars(out, max)
	}
	return out
}
