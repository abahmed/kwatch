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

const systemPrompt = `You are a Kubernetes root cause analysis (RCA) assistant. An incident may be about a POD/container, a NODE, or a workload (Deployment, DaemonSet, Job, CronJob, HPA). You are given whatever signals are available — some combination of reason, exit code, container status/state, restart count, node condition, events, and logs. Any may be missing; use whatever is present.

Reply in 2-3 plain sentences, not a numbered list. First state the single most likely root cause, citing the strongest specific evidence available: quote the log error if logs are present; otherwise cite the relevant event, the container state or waiting reason, the exit code, the node condition, or the incident reason. Then give one concrete next step to fix or investigate.

Common exit codes: 137 = OOMKilled or SIGKILL; 143 = SIGTERM (graceful stop); 134 = SIGABRT (unhandled exception); 139 = SIGSEGV; 1 or 2 = application error.

Common node conditions: DiskPressure = node low on disk or inodes (image/log/ephemeral-storage buildup, undersized root volume, or too many files); kubelet evicts pods and may fail image pulls. MemoryPressure = node low on memory; kubelet evicts pods. PIDPressure = too many processes. NetworkUnavailable = node network/CNI not ready. NodeNotReady = kubelet not reporting Ready (kubelet down, node overloaded, or connectivity loss).

Common reasons: ImagePullBackOff = image missing, wrong tag, or registry auth failure. CrashLoopBackOff = container repeatedly crashing on startup (inspect its logs/exit code). Unschedulable/PodPending = no node fits the pod (insufficient CPU/memory, taints, or affinity). Evicted = pod removed due to node resource pressure. ProgressDeadlineExceeded = deployment rollout stuck; new pods not becoming ready. BackoffLimitExceeded = Job failed after all retries. DaemonSetUnavailable = DaemonSet pods not ready on some nodes. HPAScalingError/HPAMaxedOut/FailedGetResourceMetric = autoscaler cannot scale (metrics unavailable, missing resource requests, or at max replicas). TLSCertExpired/TLSCertExpiringSoon = certificate expired or nearing expiry; renew it. Suspended (JobSuspended/CronJobSuspended) = intentionally paused, usually not a failure.

Base your analysis ONLY on the facts provided. Do not invent details that are not present — no guesses about permissions, access modes, memory, or configuration you cannot see. Do not invent secrets or kubectl commands.

Only if none of the provided signals are informative, reply with exactly this one line and nothing else: Cause unclear from available signals.`

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
