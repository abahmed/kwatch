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
	RequestTimeout = 120 * time.Second
	maxLogChars    = 6000
	maxEventChars  = 2000
)

const systemPrompt = `You are a Kubernetes root cause analysis assistant. You are called for a new incident whose reason alone is not sufficiently self-explanatory — logs and events are the primary signal.

Example:
  Logs: "panic: runtime error: invalid memory address or nil pointer dereference"
  Nil pointer dereference in application code

Analyze in this order (stop at first match):
1. Log errors, exceptions, stack traces — these ARE the root cause, quote the error
2. Kubernetes events if logs are not informative

Important: Liveness/readiness probe failures and "connection refused" to a pod's own address are symptoms — the app never started. Never report them as root cause. Find the real reason in logs.

Root cause explains WHY it failed (bug, config error, dependency issue), not WHAT happened (pod crashed, container restarted).

Output exactly 1-2 sentences describing the root cause without any prefix or label. Nothing else.

If no useful signal exists respond with: Unclear from available signals

Base your analysis only on the evidence shown below.`

func (c *Client) buildMessages(inc *model.Incident) []chatMessage {
	return []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: c.userPrompt(inc)},
	}
}

func (c *Client) userPrompt(inc *model.Incident) string {
	logs := c.redactor.scrub(selectRelevant(inc.Logs, maxLogChars))
	events := c.redactor.scrub(tailChars(inc.Events, maxEventChars))
	var b strings.Builder

	fmt.Fprintf(&b, "Reason: %s\n", c.redactor.scrub(inc.Reason))
	fmt.Fprintf(&b, "Workload: %s\nKind: %s\nNamespace: %s\n",
		c.redactor.scrub(inc.Name), c.redactor.scrub(inc.OwnerKind), inc.Namespace)
	if inc.NodeName != "" {
		fmt.Fprintf(&b, "Node: %s\n", inc.NodeName)
	}
	if inc.ContainerName != "" {
		fmt.Fprintf(&b, "Container: %s\n", c.redactor.scrub(inc.ContainerName))
	}
	if events != "" {
		fmt.Fprintf(&b, "\n--- Events ---\n%s\n", events)
	}
	if logs != "" {
		fmt.Fprintf(&b, "\n--- Logs ---\n%s\n", logs)
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
