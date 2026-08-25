package message

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/model"
)

func extractMemoryLimit(inc *model.Incident) string {
	hint := inc.Hint
	// Look for "memory limit: X" pattern in hint
	if idx := strings.Index(hint, "memory limit: "); idx >= 0 {
		start := idx + len("memory limit: ")
		sub := hint[start:]
		end := strings.IndexAny(sub, " )]")
		if end > 0 {
			return sub[:end]
		}
		return sub
	}
	return ""
}

func extractOOMTimeline(inc *model.Incident) string {
	hint := inc.Hint
	// Look for timeline in brackets: "[1,2,3]"
	if idx := strings.LastIndex(hint, "["); idx >= 0 {
		if end := strings.Index(hint[idx:], "]"); end > 0 {
			return hint[idx : idx+end+1]
		}
	}
	return ""
}

// extractOOMLeakStats parses "OOMKilled N times in Xm ..." from the hint,
// returning the kill count and the observation window in minutes.
func extractOOMLeakStats(hint string) (count, windowMin int) {
	if m := oomTimesRe.FindStringSubmatch(hint); len(m) == 3 {
		count, _ = strconv.Atoi(m[1])
		windowMin, _ = strconv.Atoi(m[2])
	}
	return count, windowMin
}

var oomTimesRe = regexp.MustCompile(`(\d+)\s+times in\s+(\d+)m`)

func extractRegistryHint(inc *model.Incident) string {
	msg := inc.LastContainerState
	if msg == nil {
		return ""
	}
	return msg.Msg
}

func extractPullSecrets(inc *model.Incident) bool {
	return strings.Contains(inc.Hint, "imagePullSecrets is configured")
}

func extractProbeEndpoint(inc *model.Incident) string {
	hint := inc.Hint
	// Look for "HTTP GET" or "TCP check" or "exec" pattern
	if idx := strings.Index(hint, "HTTP GET "); idx >= 0 {
		end := strings.IndexByte(hint[idx:], ')')
		if end > 0 {
			return hint[idx : idx+end+1]
		}
	}
	if idx := strings.Index(hint, "TCP check "); idx >= 0 {
		end := strings.IndexByte(hint[idx:], ')')
		if end > 0 {
			return hint[idx : idx+end+1]
		}
	}
	if idx := strings.Index(hint, "exec "); idx >= 0 {
		end := strings.IndexByte(hint[idx:], ')')
		if end > 0 {
			return hint[idx : idx+end+1]
		}
	}
	return ""
}

func extractSchedulingDelay(inc *model.Incident) string {
	hint := inc.Hint
	if idx := strings.Index(hint, "unschedulable for "); idx >= 0 {
		end := strings.Index(hint[idx:], " —")
		if end > 0 {
			return hint[idx+len("unschedulable for ") : idx+end]
		}
		return hint[idx+len("unschedulable for "):]
	}
	return ""
}

func extractResourceRequestStrings(inc *model.Incident) []string {
	hint := inc.Hint
	var requests []string
	for _, part := range strings.Split(hint, "; ") {
		if strings.HasSuffix(part, " requests:") || strings.Contains(part, " requests: cpu=") || strings.Contains(part, " requests: mem=") {
			requests = append(requests, strings.TrimSpace(part))
		}
	}
	return requests
}
