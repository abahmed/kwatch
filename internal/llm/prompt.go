package llm

import (
	"fmt"
	"regexp"
	"sort"
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

const systemPrompt = `You are a Kubernetes root cause analysis (RCA) assistant. An incident may concern a pod/container, a node, or a workload controller. You are given whatever signals are available — reason, exit code, container or node status, restart count, events, and logs; any may be missing.

Find the single most likely root cause, in this priority:
1. If the logs contain an error, exception, or stack trace, that IS the root cause — quote it. The exit code, restart count, OOM hints, and probe failures are downstream symptoms of it, not the cause.
2. Otherwise use the most specific signal available: the event, the container/node status or waiting reason, or the exit code.

Treat failed liveness/readiness probes and connection errors to a pod's own address as symptoms of the app not starting — never the root cause; look to the logs for why.

Reply in 2-3 plain sentences, not a list: the most likely root cause, then one concrete next step to fix or investigate it.

Use only the facts provided. Do not invent details you cannot see — permissions, memory, configuration, secrets, or commands.

Only when there is no error and no informative signal, reply with exactly this line and nothing else: Cause unclear from available signals.`

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
	fmt.Fprintf(&b, "Workload: %s\nOwnerKind: %s\nNamespace: %s\n",
		c.redactor.scrub(inc.Name), c.redactor.scrub(inc.OwnerKind), inc.Namespace)
	if inc.ContainerName != "" {
		fmt.Fprintf(&b, "Container: %s\n", c.redactor.scrub(inc.ContainerName))
	}
	fmt.Fprintf(&b, "RestartCount: %d\n", inc.RestartCount)
	if inc.LastContainerState != nil {
		fmt.Fprintf(&b, "ExitCode: %d\n", inc.LastContainerState.ExitCode)
		if inc.LastContainerState.Reason != "" {
			fmt.Fprintf(&b, "ContainerStatus: %s\n", inc.LastContainerState.Reason)
		}
		if inc.LastContainerState.Status != "" {
			fmt.Fprintf(&b, "ContainerState: %s\n", inc.LastContainerState.Status)
		}
		if inc.LastContainerState.Msg != "" {
			fmt.Fprintf(&b, "ContainerMessage: %s\n", inc.LastContainerState.Msg)
		}
	}
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
	if inc.Runbook != "" {
		fmt.Fprintf(&b, "Runbook: %s\n", c.redactor.scrub(inc.Runbook))
	}
	if inc.Hint != "" {
		fmt.Fprintf(&b, "Rule-based hint: %s\n", c.redactor.scrub(inc.Hint))
	}
	if inc.SuppressedPods > 0 {
		s := fmt.Sprintf("SuppressedPods: %d", inc.SuppressedPods)
		if len(inc.SuppressedOwners) > 0 {
			parts := make([]string, 0, len(inc.SuppressedOwners))
			for o, c := range inc.SuppressedOwners {
				parts = append(parts, fmt.Sprintf("%s (%d)", o, c))
			}
			sort.Strings(parts)
			s += " across: " + strings.Join(parts, ", ")
		}
		s += " — dependent pod alerts hidden; this incident may be the root cause\n"
		fmt.Fprint(&b, s)
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
