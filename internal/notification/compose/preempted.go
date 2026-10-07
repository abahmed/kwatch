package compose

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightPreempted ranks the preemption above generic proof: it is the
// reason the pod went away.
const weightPreempted = 0.76

// preemptedSentences say which pods the scheduler preempted, for whom,
// and when. The preemptor and the priorities are the scheduler's own
// words, read by the detector; nothing is guessed here.
func preemptedSentences(f caseFacts) []sentence {
	var victims []detection.Finding
	for _, m := range f.members {
		if m.Reason == reasons.PodPreempted {
			victims = append(victims, m)
		}
	}
	if len(victims) == 0 {
		return nil
	}
	first := victims[0]
	by := evidence(first, detection.EvidencePreemptor)
	var subject string
	if len(victims) == 1 {
		subject = first.Entity.Name + " was"
	} else {
		subject = strconv.Itoa(len(victims)) + " pods were"
	}
	text := subject + " preempted"
	if by != "" {
		text += " by " + by
	}
	if ranks := evidence(first, detection.EvidencePriorities); ranks != "" {
		text += " (priority " + ranks + ")"
	}
	if at, err := time.Parse(time.RFC3339,
		evidence(latestPreempted(victims),
			detection.EvidencePreemptedAt)); err == nil {
		text += " at " + clock(at)
	}
	text += stuckReplacements(f)
	return []sentence{{part: partProof, weight: weightPreempted,
		text: endSentence(text)}}
}

// stuckReplacements say how many pods of the victims' workload wait for
// a node, with the scheduler's own words, so the preemption and the
// pods it left without room read as one story: ", and 2 replacements
// cannot be scheduled: "0/2 nodes are available...""; nothing when all
// were placed.
func stuckReplacements(f caseFacts) string {
	waiting, said := 0, ""
	for _, m := range f.members {
		if m.Reason != reasons.Unschedulable {
			continue
		}
		waiting++
		if said == "" {
			said = evidence(m, "scheduler")
		}
	}
	if waiting == 0 {
		return ""
	}
	text := ", and " + strconv.Itoa(waiting) + " replacements cannot be " +
		"scheduled"
	if waiting == 1 {
		text = ", and 1 replacement cannot be scheduled"
	}
	if said != "" {
		text += ": " + quoted(said)
	}
	return text
}

// latestPreempted is the victim preempted last.
func latestPreempted(victims []detection.Finding) detection.Finding {
	latest := victims[0]
	for _, v := range victims[1:] {
		if v.Since.After(latest.Since) {
			latest = v
		}
	}
	return latest
}
