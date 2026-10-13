package compose

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// predictionWords are what a joined sentence must never say: it states
// what happened, not what may happen.
var predictionWords = regexp.MustCompile(
	`(?i)\b(fragile|risk|weakness|could|would|will|may|likely|soon)\b`)

func ev(label, value string) detection.Evidence {
	return detection.Evidence{Label: label, Value: value}
}

// whyCase is an incident of one failing container of deployment api,
// failing since 10:05 with the given evidence, and the given changes.
func whyCase(
	reason string, evidence []detection.Evidence, changes ...inventory.Change,
) caseFacts {
	container := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	p := incident.Incident{ID: "w1", Root: placeRoot,
		Members: members(detection.Finding{Entity: container,
			Reason: reason, Severity: detection.Critical,
			Since:    time.Date(2024, 1, 15, 10, 5, 0, 0, time.UTC),
			Evidence: evidence})}
	return caseFacts{now: writerNow, p: p, members: sortedMembers(p),
		changes: changes}
}

func releaseAt(minute int, revision string) inventory.Change {
	return inventory.Change{Entity: placeRoot, Revision: revision,
		At: time.Date(2024, 1, 15, 10, minute, 0, 0, time.UTC),
		Fields: []inventory.FieldChange{{
			Path: "containers[app].image", After: "api:1.5"}}}
}

var oomEvidence = []detection.Evidence{
	ev(detection.EvidenceMemoryLimit, "256Mi"),
	ev(detection.EvidenceMemoryPeak, "255Mi")}

func TestWhyJoinsTheMemoryNumbersWithTheReleaseBefore(t *testing.T) {
	f := whyCase(reasons.OOMKilled, oomEvidence, releaseAt(2, "14"))

	got := whySentences(f)

	assert.Len(t, got, 1)
	assert.Equal(t, "It was killed at its 256Mi memory limit; it used "+
		"255Mi at peak in the last 24 hours; it has failed since 10:05, "+
		"3m after the 10:02 release (revision 14).", got[0].text)
	assert.False(t, predictionWords.MatchString(got[0].text), got[0].text)
}

func TestWhyJoinsTheLivenessNumbersWithTheChangeBefore(t *testing.T) {
	f := whyCase(reasons.LivenessKilled, []detection.Evidence{
		ev(detection.EvidenceLivenessGives, "40s"),
		ev(detection.EvidenceUsualStart, "75s"),
		ev(detection.EvidenceStartSamples, "5")}, releaseAt(4, ""))

	got := whySentences(f)

	assert.Len(t, got, 1)
	assert.Equal(t, "Liveness allows 40s; it normally needs about 75s to "+
		"become ready (last 5 starts); it has failed since 10:05, 1m after "+
		"the 10:04 release.", got[0].text)
}

func TestWhyTakesTheBlamedChangeOfTheCause(t *testing.T) {
	f := whyCase(reasons.OOMKilled, oomEvidence)
	blamed := releaseAt(1, "")
	f.p.Cause = &rootcause.CauseRecord{Rule: "rollout", Change: &blamed}

	assert.Len(t, whySentences(f), 1)
}

func TestWhySaysNothingWithoutAChangeBeforeTheFailure(t *testing.T) {
	assert.Empty(t, whySentences(whyCase(reasons.OOMKilled, oomEvidence)))

	late := releaseAt(9, "")
	assert.Empty(t, whySentences(whyCase(reasons.OOMKilled, oomEvidence,
		late)), "a change after the failure began is not what came before")

	old := releaseAt(2, "")
	old.At = old.At.Add(-3 * time.Hour)
	assert.Empty(t, whySentences(whyCase(reasons.OOMKilled, oomEvidence,
		old)), "a change hours before is outside the window")

	other := releaseAt(2, "")
	other.Entity = inventory.CoreID(kube.KindDeployment, "shop", "web")
	assert.Empty(t, whySentences(whyCase(reasons.OOMKilled, oomEvidence,
		other)), "another object's change is not this one's")
}

func TestWhySaysNothingWithoutNumbers(t *testing.T) {
	f := whyCase(reasons.LivenessKilled, []detection.Evidence{
		ev(detection.EvidenceLivenessGives, "40s")}, releaseAt(2, ""))

	assert.Empty(t, whySentences(f))
}

func TestWhySaysNothingForAKillByTheNode(t *testing.T) {
	evidence := append([]detection.Evidence{
		ev(detection.EvidenceKilledByNode, "node memory pressure")},
		oomEvidence...)

	assert.Empty(t, whySentences(whyCase(reasons.OOMKilled, evidence,
		releaseAt(2, ""))))
}

func TestWhySaysNothingForAnAdvisoryAlone(t *testing.T) {
	f := whyCase(reasons.OOMKilled, oomEvidence, releaseAt(2, ""))
	for key, m := range f.p.Members {
		m.Advisory = true
		f.p.Members[key] = m
	}
	f.members = sortedMembers(f.p)

	assert.Empty(t, whySentences(f))
}

func TestWhyReplacesTheMemoryProofItJoins(t *testing.T) {
	f := whyCase(reasons.OOMKilled, oomEvidence, releaseAt(2, ""))

	joined := arrange(writeAll(f,
		[]sentenceWriter{memorySentences, whySentences}))
	require.Len(t, joined, 1)
	assert.Equal(t, partConsequence, joined[0].part)
	assert.Empty(t, whySentences(whyCase(reasons.OOMKilled, oomEvidence)))
	assert.NotEmpty(t, memorySentences(
		whyCase(reasons.OOMKilled, oomEvidence)))
}

func TestWhyReachesTheNoteOnce(t *testing.T) {
	f := whyCase(reasons.OOMKilled, oomEvidence, releaseAt(2, "14"))
	f.p.Members = members(f.members[0])

	note := Writer{}.Write(incident.Decision{Action: incident.Announce,
		Incident: f.p, Facts: incident.Facts{Changes: f.changes}},
		writerNow).Note

	assert.Contains(t, note, "it has failed since 10:05, 3m after the "+
		"10:02 release (revision 14).")
	assert.Equal(t, 1, countOf(note, "memory limit"))
}

func countOf(text, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(text); i++ {
		if text[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}
