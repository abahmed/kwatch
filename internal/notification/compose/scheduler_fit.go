package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
)

// weightSchedulerFit puts the node-by-node verdict right after the
// numbers (weightSchedulerNumbers) and before the scheduler's quote.
const weightSchedulerFit = weightScheduler + 0.05

// fitValues returns the values of one evidence label across members.
func fitValues(f caseFacts, label string) []string {
	var out []string
	for _, s := range f.members {
		for _, e := range s.Evidence {
			if v := strings.TrimSpace(e.Value); e.Label == label && v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// fitSentences say which nodes could take an unschedulable pod and what
// stops the rest: "No node fits: pool general has no node with 1.5 CPU
// free (best: n3 has 1.2 CPU free); pool gpu has room but taint ... isn't
// tolerated. Tolerating ... would fit it on gpu-1." Nothing when the
// finding carries no verdict.
func fitSentences(f caseFacts) []sentence {
	var text string
	switch blocked := fitValues(f, detection.EvidenceFit); {
	case len(blocked) > 0:
		text = "No node fits: " + strings.Join(blocked, "; ") + "."
	default:
		return fitNoteSentences(f)
	}
	for _, would := range fitValues(f, detection.EvidenceFitWould) {
		text += " " + upperFirst(would) + "."
	}
	for _, note := range fitValues(f, detection.EvidenceFitNote) {
		text += " (Note: " + note + ".)"
	}
	return []sentence{{part: partProof, weight: weightSchedulerFit,
		text: text}}
}

// fitNoteSentences say that the check was skipped.
func fitNoteSentences(f caseFacts) []sentence {
	notes := fitValues(f, detection.EvidenceFitNote)
	if len(notes) == 0 {
		return nil
	}
	return []sentence{{part: partProof, weight: weightSchedulerFit,
		text: upperFirst(notes[0]) + "."}}
}

// autoscalerSentences say what the autoscaler told the pod, in its own
// words: it is adding a node, or it cannot.
func autoscalerSentences(f caseFacts) []sentence {
	states := fitValues(f, detection.EvidenceAutoscaler)
	if len(states) == 0 {
		return nil
	}
	said := ""
	if quotes := fitValues(f, detection.EvidenceAutoscalerSays); len(quotes) > 0 {
		said = quoted(quotes[0])
	}
	text := "The autoscaler is adding a node for it"
	switch states[0] {
	case detection.AutoscalerBlocked:
		text = "The autoscaler can't add a node for it"
	case detection.AutoscalerLate:
		text = "The autoscaler said it was adding a node for it, but " +
			"none has taken it"
	}
	if said != "" {
		text += ": " + said
	}
	return []sentence{{part: partConsequence, weight: weightSchedulerFit,
		text: endSentence(text)}}
}
