package pipeline

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxInvestigatedContainers bounds log reads per crash investigation.
// Replicas usually fail the same way; three are enough to see that.
const maxInvestigatedContainers = 3

// readCrash reads the previous run of the incident's crashing
// containers: the termination message from the model, the first
// meaningful error line and a short excerpt from the logs. Replicas that
// fail with the same signature are shown once.
func readCrash(
	ctx context.Context, s Sources, p incident.Incident,
) Result {
	var r Result
	seen := map[signatureKey]bool{}
	add := func(fact, text string) {
		key := signatureKey{fact: fact, signature: logSignature(text)}
		if text != "" && !seen[key] {
			seen[key] = true
			r.Evidence = append(r.Evidence,
				incident.Fact{Kind: fact, Text: text})
		}
	}
	for _, c := range topContainers(p) {
		if ctx.Err() != nil {
			break
		}
		// The termination message is in the model: no API call.
		add(incident.FactTermination,
			attributeText(s.Model, c.Entity, kube.AttrLastMessage))
		if s.Logs == nil {
			continue
		}
		lines := s.Logs(ctx, c.Entity)
		add(incident.FactError, firstErrorLine(lines))
		r.Output = appendUnique(r.Output, kube.ErrorExcerpt(lines), seen)
	}
	return r
}

// signatureKey identifies one fact or output line by its log
// signature, so replicas that fail the same way are shown once.
type signatureKey struct {
	fact, signature string
}

// outputFact is the signatureKey fact of quoted output lines.
const outputFact = "output"

// appendUnique appends the lines whose signature is not yet in seen.
func appendUnique(
	out, lines []string, seen map[signatureKey]bool,
) []string {
	for _, line := range lines {
		key := signatureKey{fact: outputFact, signature: logSignature(line)}
		if !seen[key] {
			seen[key] = true
			out = append(out, line)
		}
	}
	return out
}

// stackFrame matches lines that only say where an error happened: Java
// and JavaScript "at" frames, Python "File" lines, Go goroutine headers
// and source positions, and elided-frame markers.
var stackFrame = regexp.MustCompile(`^(at \S+|File ".*", line \d+|` +
	`goroutine \d+ \[|\S*\.(go|py|js|ts|java|rb|rs):\d+|` +
	`\.\.\. \d+ more|Traceback \(most recent call last\)|#\d+ )`)

// firstErrorLine is the first line that names a failure and is not a
// stack frame. The first error is the cause; the last frame of a trace
// only says where the program noticed it.
func firstErrorLine(lines []string) string {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if kube.IsErrorLine(line) && !stackFrame.MatchString(line) {
			return line
		}
	}
	return ""
}

// signatureRules replace what differs between occurrences of one error:
// timestamps, IDs, addresses and numbers. Order matters: a UUID is
// replaced before its digits are.
var signatureRules = []struct {
	pattern *regexp.Regexp
	with    string
}{
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}` +
		`(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`), "<time>"},
	{regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}(\.\d+)?\b`), "<time>"},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-` +
		`[0-9a-f]{4}-[0-9a-f]{12}\b`), "<id>"},
	{regexp.MustCompile(`(?i)\b0x[0-9a-f]+\b`), "<addr>"},
	{regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}(:\d+)?\b`), "<ip>"},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{12,}\b`), "<id>"},
	// Pod names of a ReplicaSet: name-<hash>-<suffix>.
	{regexp.MustCompile(`-[a-z0-9]{8,10}-[a-z0-9]{5}\b`), "-<pod>"},
	{regexp.MustCompile(`\b\d+\b`), "<n>"},
	{regexp.MustCompile(`\s+`), " "},
}

// logSignature normalizes a log line so the same error from different
// replicas, runs or requests compares equal.
func logSignature(line string) string {
	for _, rule := range signatureRules {
		line = rule.pattern.ReplaceAllString(line, rule.with)
	}
	return strings.TrimSpace(line)
}

// topContainers returns the incident's most severe container findings,
// at most maxInvestigatedContainers of them, each container once.
func topContainers(p incident.Incident) []detection.Finding {
	var containers []detection.Finding
	for key, s := range p.Members {
		if key.Entity.Kind == kube.KindContainer {
			containers = append(containers, s)
		}
	}
	sort.Slice(containers, func(i, j int) bool {
		if containers[i].Severity != containers[j].Severity {
			return containers[i].Severity > containers[j].Severity
		}
		return containers[i].Entity.String() <
			containers[j].Entity.String()
	})
	var out []detection.Finding
	seen := map[inventory.EntityID]bool{}
	for _, c := range containers {
		if !seen[c.Entity] && len(out) < maxInvestigatedContainers {
			seen[c.Entity] = true
			out = append(out, c)
		}
	}
	return out
}
