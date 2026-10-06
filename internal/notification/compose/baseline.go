package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// weightBaseline ranks the comparison with the workload's own past: it
// is what tells a reader whether the failure is news, so it sits above
// the memory history.
const weightBaseline = 0.7

// baselineSentences compare a finding with what its workload did over
// the last week, from the evidence the detectors attached. They only
// describe the past: "restarts 12×/h vs a usual 0.1×/h".
func baselineSentences(f caseFacts) []sentence {
	var phrases []string
	unusual, usual := false, false
	for _, m := range f.members {
		for _, e := range m.Evidence {
			if e.Label == detection.EvidenceBaseline {
				phrases = append(phrases, e.Value)
			}
		}
		unusual = unusual || m.Normal == detection.NormalUnusual
		usual = usual || m.Normal == detection.NormalUsual
	}
	if len(phrases) == 0 {
		return nil
	}
	lead := "Over the last week of this workload: "
	switch {
	case unusual:
		lead = "That is unusual for this workload: "
	case usual:
		lead = "That is within what this workload normally does: "
	}
	text := lead + strings.Join(phrases, "; ") + "."
	return []sentence{{part: partProof, weight: weightBaseline, text: text}}
}

// usualTail is what a list line adds for an incident the workload's
// normal excused: "(within its normal: restarts 2×/h vs a usual 2×/h)".
// It is empty for any other incident.
func usualTail(d incident.Decision) string {
	for _, m := range sortedMembers(d.Incident) {
		if m.Normal != detection.NormalUsual {
			continue
		}
		for _, e := range m.Evidence {
			if e.Label == detection.EvidenceBaseline {
				return " (within its normal: " + e.Value + ")"
			}
		}
	}
	return ""
}

// withUsualTail puts the tail before the full stop of a sentence title.
func withUsualTail(title string, d incident.Decision) string {
	tail := usualTail(d)
	if tail == "" {
		return title
	}
	return strings.TrimSuffix(title, ".") + tail + "."
}
