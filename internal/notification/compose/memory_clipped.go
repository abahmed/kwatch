package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/notification"
)

// clippedShare is how close to its limit a peak counts as held at the
// limit: within 5%, as the detector's "hours near limit" counts it.
const clippedShare = 0.95

// usageClipped reports a container whose memory use was held at its
// limit, so what it really needs is not known: its observed peak is
// within 5% of the limit (runs that each ended near the limit peak
// there too), or its history has hours near the limit. A peak that is
// not known counts as clipped: nothing shows headroom. The sizing
// sentence (sizingText) and the memory fix (memoryFix) both ask it.
func usageClipped(m detection.Finding) bool {
	if evidence(m, detection.EvidenceMemoryNearLimit) != "" {
		return true
	}
	limit := mebibytes(evidence(m, detection.EvidenceMemoryLimit))
	peak := mebibytes(evidence(m, detection.EvidenceMemoryPeak))
	return peak <= 0 || limit <= 0 || peak >= clippedShare*limit
}

// clippedMemoryStep is the memory fix of a container whose use was held
// at its limit: raise it and watch the next runs, with no number, since
// any number would be a guess. A LimitRange already at the limit, or a
// quota with no memory left, has to be raised first, and the step says
// so.
func clippedMemoryStep(
	container string, m detection.Finding,
) notification.Step {
	limit := evidence(m, detection.EvidenceMemoryLimit)
	text := "Raise the memory limit of container " + container +
		" and watch its next runs: its use was held at its " + limit +
		" limit, so how much more it needs is not known (changes the " +
		"cluster)"
	now := mebibytes(limit)
	if most := mebibytes(evidence(m,
		detection.EvidenceMemoryMaxAllowed)); most > 0 && most <= now {
		text += "; the namespace's LimitRange caps memory at " +
			mebiText(most) + " per container, so raise that first"
	}
	if left := evidence(m, detection.EvidenceMemoryQuotaLeft); left != "" &&
		mebibytes(left) <= 0 {
		text += "; the namespace's ResourceQuota has no limits.memory " +
			"left, so raise that first"
	}
	return notification.Step{Text: text}
}
