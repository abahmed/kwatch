package compose

import (
	"math"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/notification"
)

// checkedMemoryFix checks a raised memory limit against what the
// cluster allows, as far as kwatch can read it, before it is offered:
// the namespace's LimitRange maximum, the room left under its
// ResourceQuota for every replica, and the largest node. A value the
// cluster refuses is not offered; the step says what blocks it. A
// workload a GitOps controller deploys is changed in Git, where the
// next sync would otherwise undo the change.
func checkedMemoryFix(
	step notification.Step, m detection.Finding, app string,
) notification.Step {
	target := limitInStep(step.Command)
	if target <= 0 {
		return step
	}
	if blocked := memoryBlock(m, target); blocked != "" {
		return notification.Step{Text: blocked}
	}
	if unread := evidence(m, detection.EvidenceBoundsUnread); unread != "" {
		step.Text += "; not checked against the namespace's " + unread +
			", which kwatch cannot read"
	}
	if tool, _, _ := strings.Cut(app, "/"); tool == "argocd" ||
		tool == "flux" {
		return notification.Step{Text: "Change the memory limit to " +
			mebiText(target) + " in Git (" + app + "): a change in " +
			"the cluster is undone by the next sync"}
	}
	return step
}

// memoryBlock says what refuses a limit of target MiB, or "".
func memoryBlock(m detection.Finding, target float64) string {
	limit := mebibytes(evidence(m, detection.EvidenceMemoryLimit))
	want := mebiText(target)
	if most := mebibytes(evidence(m,
		detection.EvidenceMemoryMaxAllowed)); most > 0 && target > most {
		return "The namespace's LimitRange allows at most " +
			mebiText(most) + " of memory per container, below " + want +
			"; raise that limit first, or find what uses the memory"
	}
	replicas := 1.0
	if n, err := strconv.Atoi(evidence(m,
		detection.EvidenceReplicas)); err == nil && n > 1 {
		replicas = float64(n)
	}
	need := (target - limit) * replicas
	if left := evidence(m, detection.EvidenceMemoryQuotaLeft); left != "" &&
		need > mebibytes(left) {
		return "The namespace's ResourceQuota has " + left +
			" of limits.memory left; a " + want + " limit needs " +
			mebiText(need) + " more; raise the quota first"
	}
	if largest := mebibytes(evidence(m,
		detection.EvidenceMemoryNodeMax)); largest > 0 && target > largest {
		return "No node has " + want + " of allocatable memory (the " +
			"largest has " + mebiText(largest) + "); add a larger node " +
			"first, or find what uses the memory"
	}
	return ""
}

// limitInStep reads the MiB a "--limits=memory=768Mi" command sets.
func limitInStep(command string) float64 {
	_, value, ok := strings.Cut(command, "--limits=memory=")
	if !ok {
		return 0
	}
	return mebibytes(strings.Fields(value + " ")[0])
}

// mebiText writes MiB as "768Mi".
func mebiText(mi float64) string {
	return strconv.FormatFloat(math.Ceil(mi), 'f', -1, 64) + "Mi"
}

// cappedFix adds to a fix line that raises a memory limit what blocks
// the raise: a LimitRange that already caps the container at the limit
// it has. Any other fix line is kept as it is.
func cappedFix(f caseFacts, place detection.Place, text string) string {
	if place.Path != "resources.limits.memory" {
		return text
	}
	now := mebibytes(place.Value)
	for _, m := range failing(f.members) {
		most := mebibytes(evidence(m, detection.EvidenceMemoryMaxAllowed))
		if most > 0 && now > 0 && most <= now {
			return strings.TrimSuffix(text, ".") + "; the namespace's " +
				"LimitRange allows at most " + mebiText(most) +
				" per container, so that cap has to be raised first."
		}
	}
	return text
}
