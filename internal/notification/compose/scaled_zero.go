package compose

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// weightScaledZero ranks who scaled a workload to zero: it is the whole
// proof of a scale-down that is not an outage of the workload itself.
const weightScaledZero = 0.7

// scaledZeroSentences say who scaled a routed workload to zero and
// when, as far as the change history knows: "It was scaled to 0 by
// kubectl-scale at 21:10." The actor is the field manager's name, shown
// as recorded.
func scaledZeroSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		if m.Reason != reasons.ScaledToZeroRouted {
			continue
		}
		by := evidence(m, detection.EvidenceScaledBy)
		at, err := time.Parse(time.RFC3339,
			evidence(m, detection.EvidenceScaledAt))
		text := "It was scaled to 0"
		if by != "" {
			text += " by " + by
		}
		if err == nil {
			text += " at " + clock(at)
		}
		if by == "" && err != nil {
			return nil
		}
		return []sentence{{part: partProof, weight: weightScaledZero,
			text: text + "."}}
	}
	return nil
}

// staleChange reports members that no change explains any more: what
// they say now is that a workload is scaled to zero or paused with an
// edit, whatever was blamed when the incident began. Neither the lead
// nor the steps blame or undo the change then: the lead states what
// holds now, and no step reverts a release nothing fails under.
func staleChange(members []detection.Finding) bool {
	return onlyScaledToZero(members) || onlyPausedEdit(members)
}

// onlyScaledToZero reports members that only say a routed workload is
// scaled to zero. Such a workload is not failing, and no release is
// blamed for it: the lead states the scale-down (its own finding), and
// scaledZeroSentences say when and by whom.
func onlyScaledToZero(members []detection.Finding) bool {
	found := false
	for _, m := range failing(members) {
		if m.Reason != reasons.ScaledToZeroRouted {
			return false
		}
		found = true
	}
	return found
}

// onlyPausedEdit reports members that only say a Deployment is paused
// with an edit that has not rolled out. Nothing is failing, and the
// release is not blamed: the edit is what waits.
func onlyPausedEdit(members []detection.Finding) bool {
	found := false
	for _, m := range failing(members) {
		if m.Reason != reasons.DeploymentPausedEdit {
			return false
		}
		found = true
	}
	return found
}

// pausedSteps resume a Deployment that is paused with an edit waiting.
func pausedSteps(root inventory.EntityID) []notification.Step {
	return []notification.Step{{
		Text: "Roll out the edit by resuming the Deployment " +
			"(changes the cluster)",
		Command: "kubectl rollout resume deploy/" + quote(root.Name) +
			namespaceFlag(root.Namespace),
		Mutating: true,
	}}
}
