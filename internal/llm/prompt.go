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

const systemPrompt = `You are a Kubernetes root cause analysis assistant.
Identify the root cause and a next step from the incident below.

Example:
  Incident: CrashLoopBackOff, exit code 137, container OOMKilled
  Logs: "java.lang.OutOfMemoryError: Java heap space"
  Root cause: Java heap exhausted (OutOfMemoryError) causing OOM kill
  Next step: Increase -Xmx JVM arg and review application memory usage

Analyze in this order (stop at first match):
1. Log errors, exceptions, stack traces — these ARE the root cause, quote the error
2. ContainerStatus (OOMKilled, ImagePullBackOff, CrashLoopBackOff, Error) and exit code
3. Kubernetes events if logs and status are not informative
4. Restart count as supporting context only

Important: Liveness/readiness probe failures and "connection refused" to a pod's own address are symptoms — the app never started. Never report them as root cause. Find the real reason in logs.

Root cause explains WHY it failed (memory leak, nil pointer, missing dependency), not WHAT happened (pod crashed, container restarted).

Output exactly two lines:
Root cause: <cause with supporting evidence>
Next step: <specific investigation or fix>

If no useful signal exists:
Root cause: Unclear from available signals
Next step: Inspect complete logs, recent changes, and cluster events

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

	fmt.Fprintf(&b, "--- Summary ---\n")
	fmt.Fprintf(&b, "Reason: %s\n", c.redactor.scrub(inc.Reason))
	fmt.Fprintf(&b, "Workload: %s\nKind: %s\nNamespace: %s\n",
		c.redactor.scrub(inc.Name), c.redactor.scrub(inc.OwnerKind), inc.Namespace)
	if inc.NodeName != "" {
		fmt.Fprintf(&b, "Node: %s\n", inc.NodeName)
	}
	if inc.ContainerName != "" {
		fmt.Fprintf(&b, "Container: %s\n", c.redactor.scrub(inc.ContainerName))
	}
	fmt.Fprintf(&b, "Restarts: %d | Duration: %.0f min | Occurrences: %d | AffectedPods: %d\n",
		inc.RestartCount, inc.LastSeen.Sub(inc.FirstSeen).Minutes(), inc.Count, len(inc.Resources))
	if inc.LastContainerState != nil {
		if inc.LastContainerState.ExitCode != 0 {
			fmt.Fprintf(&b, "ExitCode: %d\n", inc.LastContainerState.ExitCode)
		}
		if inc.LastContainerState.Reason != "" {
			fmt.Fprintf(&b, "ContainerStatus: %s\n", inc.LastContainerState.Reason)
		}
		if inc.LastContainerState.Msg != "" {
			fmt.Fprintf(&b, "ContainerMessage: %s\n", inc.LastContainerState.Msg)
		}
	}
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
		fmt.Fprintf(&b, "Hint: %s\n", c.redactor.scrub(inc.Hint))
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
		s += " — dependent pod alerts hidden\n"
		fmt.Fprint(&b, s)
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
