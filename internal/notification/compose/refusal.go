package compose

import "github.com/abahmed/kwatch/internal/inventory/kube"

// weightRefusal ranks the exact numbers of the refusing quota or limit
// range: they are the proof itself, above a change that may have caused
// them.
const weightRefusal = 0.85

// refusalSentences quote the refusing object's own numbers, read from
// the API server's FailedCreate text the finding carries:
// "ResourceQuota team-a: requests.memory 31.5Gi of 32Gi used; this pod
// asks 1Gi".
func refusalSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		line := refusalLine(f, evidence(m, "event"))
		if line != "" {
			return []sentence{{part: partProof, weight: weightRefusal,
				text: line + "."}}
		}
	}
	return nil
}

// refusalLine is the quota or limit range line of one message, or "".
// A limit range is named after the incident's root when that is one.
func refusalLine(f caseFacts, message string) string {
	if line := quotaLine(message, ""); line != "" {
		return line
	}
	name := ""
	if f.p.Root.Kind == kube.KindLimitRange {
		name = f.p.Root.Name
	}
	return limitLine(message, name)
}
