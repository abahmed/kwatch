package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/notification"
)

func killAt(extra ...detection.Evidence) detection.Finding {
	return detection.Finding{Evidence: append([]detection.Evidence{
		{Label: detection.EvidenceMemoryLimit, Value: "512Mi"},
		{Label: detection.EvidenceMemoryPeak, Value: "510Mi"},
	}, extra...)}
}

var raise = notification.Step{
	Text: "Raise the memory limit to 768Mi (changes the cluster)",
	Command: "kubectl set resources 'deployment/api' -c 'app' " +
		"--limits=memory=768Mi",
	Mutating: true,
}

func TestCheckedMemoryFixKeepsAValueThatFits(t *testing.T) {
	got := checkedMemoryFix(raise, killAt(
		detection.Evidence{Label: detection.EvidenceMemoryMaxAllowed,
			Value: "1Gi"},
		detection.Evidence{Label: detection.EvidenceMemoryQuotaLeft,
			Value: "2Gi"},
		detection.Evidence{Label: detection.EvidenceMemoryNodeMax,
			Value: "8Gi"}), "")
	assert.Equal(t, raise, got)
}

func TestCheckedMemoryFixSaysWhatBlocksIt(t *testing.T) {
	for name, tc := range map[string]struct {
		evidence detection.Evidence
		want     string
	}{
		"limit range": {detection.Evidence{
			Label: detection.EvidenceMemoryMaxAllowed, Value: "512Mi"},
			"LimitRange allows at most 512Mi"},
		"quota": {detection.Evidence{
			Label: detection.EvidenceMemoryQuotaLeft, Value: "100Mi"},
			"ResourceQuota has 100Mi of limits.memory left; a 768Mi " +
				"limit needs 768Mi more"},
		"node": {detection.Evidence{
			Label: detection.EvidenceMemoryNodeMax, Value: "600Mi"},
			"No node has 768Mi"},
	} {
		got := checkedMemoryFix(raise, killAt(tc.evidence,
			detection.Evidence{Label: detection.EvidenceReplicas,
				Value: "3"}), "")
		assert.Empty(t, got.Command, name)
		assert.False(t, got.Mutating, name)
		assert.Contains(t, got.Text, tc.want, name)
	}
}

func TestCheckedMemoryFixNamesUnreadBoundsAndGitOps(t *testing.T) {
	got := checkedMemoryFix(raise, killAt(detection.Evidence{
		Label: detection.EvidenceBoundsUnread,
		Value: "LimitRange, ResourceQuota"}), "")
	assert.Contains(t, got.Text, "not checked against the namespace's "+
		"LimitRange, ResourceQuota")
	assert.NotEmpty(t, got.Command)

	git := checkedMemoryFix(raise, killAt(), "argocd/shop")
	assert.Empty(t, git.Command)
	assert.Contains(t, git.Text, "in Git (argocd/shop)")
}

func TestSizingTextStatesTheWindowItHas(t *testing.T) {
	m := killAt(
		detection.Evidence{Label: detection.EvidenceMemoryWindow,
			Value: "5h"},
		detection.Evidence{Label: detection.EvidenceMemoryRequest,
			Value: "128Mi"},
		detection.Evidence{Label: detection.EvidenceMemoryNearLimit,
			Value: "4"})
	assert.Equal(t, "In the last 5h, all the history kwatch has, its "+
		"memory peaked at 510Mi (limit 512Mi, request 128Mi); it was "+
		"near the limit in 4 of those hours, so how much more it needs "+
		"is not known.", sizingText(m))
	assert.Empty(t, sizingText(killAt()), "no window, no sizing")
}

func TestRunsSentence(t *testing.T) {
	assert.Equal(t, "Its last 3 runs each ended near 510Mi.",
		runsSentence("3 runs, each near 510Mi"))
	assert.Equal(t, "Its last 3 runs ended between 200Mi and 512Mi.",
		runsSentence("3 runs, between 200Mi and 512Mi"))
}

func TestCappedFixNamesTheLimitRange(t *testing.T) {
	place := detection.Place{Path: "resources.limits.memory",
		Value: "512Mi"}
	f := caseFacts{members: []detection.Finding{killAt(detection.Evidence{
		Label: detection.EvidenceMemoryMaxAllowed, Value: "512Mi"})}}
	assert.Equal(t, "Fix: raise it; the namespace's LimitRange allows "+
		"at most 512Mi per container, so that cap has to be raised first.",
		cappedFix(f, place, "Fix: raise it."))

	f.members = []detection.Finding{killAt()}
	assert.Equal(t, "Fix: raise it.", cappedFix(f, place, "Fix: raise it."))
}
