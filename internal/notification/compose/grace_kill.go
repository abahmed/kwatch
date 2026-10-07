package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
)

// weightGraceKill ranks the hard kill with the other facts about how a
// pod ended.
const weightGraceKill = 0.27

// graceKillSentences say how many pods did not stop within their grace
// period and were killed, and when. It speaks of a rollout only when the
// detector saw the workload mid-rollout, and quotes the kubelet's own
// words when a preStop hook or a kill failed.
func graceKillSentences(f caseFacts) []sentence {
	var killed []detection.Finding
	for _, m := range f.members {
		if m.Reason == reasons.PodKilledAtGrace {
			killed = append(killed, m)
		}
	}
	if len(killed) == 0 {
		return nil
	}
	first := killed[0]
	grace := evidence(first, detection.EvidenceGracePeriod)
	text := graceSubject(f, killed) + " did not stop within " +
		graceWords(len(killed), grace) + " and " + killedWords(len(killed)) +
		" killed (exit code 137); work in flight was cut off"
	if evidence(first, detection.EvidenceDuringRollout) == "true" {
		text = "During the rollout at " + clock(first.Since) + ", " + text
	} else {
		text = "At " + clock(first.Since) + ", " + text
	}
	out := []sentence{{part: partProof, weight: weightGraceKill,
		text: endSentence(text)}}
	if hook := evidence(first, detection.EvidenceStopHook); hook != "" {
		out = append(out, sentence{part: partProof,
			weight: weightGraceKill - 0.01,
			text:   "The kubelet reported " + quoted(hook) + "."})
	}
	return out
}

// graceSubject is "3 api pods", "1 api pod" or "3 pods".
func graceSubject(f caseFacts, killed []detection.Finding) string {
	noun := "pod"
	if len(killed) != 1 {
		noun = "pods"
	}
	name := ""
	if incident.IsWorkload(f.p.Root.Kind) {
		name = shortName(f.p.Root) + " "
	}
	return strconv.Itoa(len(killed)) + " " + name + noun
}

func graceWords(n int, grace string) string {
	if grace == "" {
		return "their grace period"
	}
	if n == 1 {
		return "its " + grace + " grace period"
	}
	return "their " + grace + " grace period"
}

func killedWords(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}
