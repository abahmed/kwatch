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
// describe the past: "restarts 12×/h vs a usual 0.1×/h". Each figure
// keeps the verdict of the finding it came from, and a verdict says
// each metric once, by the first finding with it.
func baselineSentences(f caseFacts) []sentence {
	var out []sentence
	for i, verdict := range []detection.Normality{detection.NormalUnusual,
		detection.NormalUsual, detection.NormalUnjudged} {
		var phrases []string
		said := map[string]bool{}
		for _, m := range f.members {
			for _, e := range m.Evidence {
				metric := baselineMetric(e.Value)
				if m.Normal != verdict ||
					e.Label != detection.EvidenceBaseline || said[metric] {
					continue
				}
				said[metric] = true
				phrases = append(phrases, e.Value)
			}
		}
		if len(phrases) > 0 {
			out = append(out, sentence{part: partProof,
				weight: weightBaseline - float64(i)/100,
				text: baselineLead[verdict] + strings.Join(phrases, "; ") +
					"."})
		}
	}
	return out
}

// baselineMetric is what a baseline phrase measures: the words before
// its first figure ("restarts" in "restarts 8×/h vs a usual 2×/h").
func baselineMetric(phrase string) string {
	if i := strings.IndexAny(phrase, "0123456789"); i >= 0 {
		return strings.TrimSpace(phrase[:i])
	}
	return phrase
}

// baselineLead introduces the figures of one verdict.
var baselineLead = map[detection.Normality]string{
	detection.NormalUnusual:  "That is unusual for this workload: ",
	detection.NormalUsual:    usualLead,
	detection.NormalUnjudged: "Over the last week of this workload: ",
}

const usualLead = "That is within what this workload normally does: "

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
