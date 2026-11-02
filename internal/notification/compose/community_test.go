package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func communityWriters() (full, community, quiet Writer) {
	full = Writer{}
	community = Writer{}.ForEdition(feature.CommunityEntitlements(), false)
	quiet = Writer{}.ForEdition(feature.CommunityEntitlements(), true)
	return full, community, quiet
}

func TestProEditionWritesTheFullMessage(t *testing.T) {
	p := solvedNodeOutage(t)
	want := Writer{}.Write(announce(p), at(4, 0))
	got := Writer{}.ForEdition(feature.ProEntitlements(), false).
		Write(announce(p), at(4, 0))
	assert.Equal(t, want, got)
}

func TestWriterWithoutEntitlementsIsCommunity(t *testing.T) {
	p := solvedNodeOutage(t)
	want := Writer{}.ForEdition(feature.CommunityEntitlements(), false).
		Write(announce(p), at(4, 0))
	got := Writer{}.ForEdition(nil, false).Write(announce(p), at(4, 0))
	assert.Equal(t, want, got)
	assert.Empty(t, got.Confidence, "confidence is Pro detail")
}

func TestCommunityMessageKeepsTheHeadlineAndDropsTheDetail(t *testing.T) {
	p := solvedNodeOutage(t)
	require.NotEmpty(t, p.Cause.Proof)
	full, community, _ := communityWriters()
	pro := full.Write(announce(p), at(4, 0))
	got := community.Write(announce(p), at(4, 0))

	assert.NotEmpty(t, pro.Confidence)
	assert.Empty(t, got.Confidence, "confidence is Pro detail")
	assert.Equal(t, firstSentence(pro.Note), firstSentence(got.Note),
		"the headline stays")
	assert.Less(t, len(got.Doc), len(pro.Doc))
	assert.NotContains(t, got.Note, "Checked:")
	assert.Equal(t, pro.Route, got.Route, "routing reads the incident")
	assert.NotEmpty(t, pro.Steps)
	assert.Empty(t, got.Steps, "the fix steps are Pro")
	assert.NotContains(t, got.Note, "kubectl", "the commands are Pro")
	assert.Contains(t, noteFooter(got.Note), proFix)
}

func firstSentence(note string) string {
	line, _, _ := strings.Cut(note, "\n")
	first, _, _ := strings.Cut(line, ". ")
	return strings.TrimSuffix(first, ".")
}

func TestCommunityNamesWhatProWouldAdd(t *testing.T) {
	p := solvedNodeOutage(t)
	p.Mode = detection.ModeNotReady
	_, community, quiet := communityWriters()
	d := announce(p)
	require.NotEmpty(t, d.Incident.Impact)

	got := community.Write(d, at(4, 0))
	hint := noteFooter(got.Note)
	assert.Equal(t, noteNode+" "+proFix+proInvite, hint, "%q", got.Note)
	assert.NotContains(t, strings.Join(got.Lines, "\n"), "Pro would",
		"the note is not an explanation line")
	last := got.Doc[len(got.Doc)-1]
	assert.Equal(t, notification.Small, last.Kind)

	hidden := quiet.Write(d, at(4, 0))
	assert.NotContains(t, hidden.Note, "Pro would",
		"app.hideProHints removes the note")
	assert.Equal(t, strings.TrimSuffix(got.Note, "\n"+hint), hidden.Note)
}

func TestCommunityHasNoProNoteWithoutProHints(t *testing.T) {
	p := solvedNodeOutage(t)
	e := noHints{feature.CommunityEntitlements()}
	got := Writer{}.ForEdition(e, false).Write(announce(p), at(4, 0))
	assert.NotContains(t, got.Note, "Pro:")
}

// noHints is a Community edition whose limits carry no Pro hints.
type noHints struct{ feature.Entitlements }

func (n noHints) Limits() feature.Limits {
	limits := n.Entitlements.Limits()
	limits.ProHints = false
	return limits
}

func TestCommunityResolveHasNoProNote(t *testing.T) {
	p := solvedNodeOutage(t)
	_, community, _ := communityWriters()
	d := announce(p)
	d.Action = incident.Resolve
	got := community.Write(d, at(4, 0))
	assert.NotContains(t, got.Note, "Pro:")
}

func noteFooter(note string) string {
	lines := strings.Split(note, "\n")
	return lines[len(lines)-1]
}

func TestCommunityOmitsInvestigationOutput(t *testing.T) {
	d := announce(solvedNodeOutage(t))
	d.Facts.Output = []string{"panic: boom"}
	full, community, _ := communityWriters()

	assert.Equal(t, []string{"panic: boom"}, full.Write(d, at(4, 0)).Output)
	assert.Empty(t, community.Write(d, at(4, 0)).Output)
}

func TestCommunityNamesOneCauseWhereProNamesTwo(t *testing.T) {
	p := secretCause(0.74)
	p.Cause.Rival = &rootcause.CauseRecord{
		Root:  inventory.CoreID("configmap", "default", "cfg"),
		Score: 0.72, Summary: "configmap cfg is missing"}
	full, community, _ := communityWriters()

	pro := full.Write(announce(p), writerNow)
	got := community.Write(announce(p), writerNow)

	assert.Contains(t, pro.Title, "two possible causes")
	assert.NotContains(t, got.Title, "two possible causes")
	assert.NotContains(t, got.Note, "configmap cfg")
	assert.NotContains(t, got.Note, "secret db", "only the category shows")
	assert.Equal(t, 1, strings.Count(got.Note, teaserLabel))
	require.NotNil(t, p.Cause.Rival, "the incident keeps its rival")
}

// Who made a change is Pro detail: a Community fix-attempt update and
// its resolve say what changed and when, never by whom.
func TestCommunityAttemptDoesNotNameTheActor(t *testing.T) {
	p := badRollout()
	change := inventory.Change{Entity: payments, Actor: "alice",
		Revision: "15", At: at(6, 0)}
	p.Attempt = &change
	d := announce(p)
	d.Action, d.Reason = incident.Update, incident.ReasonFixAttempt
	full, community, _ := communityWriters()

	assert.Contains(t, full.Write(d, at(7, 0)).Note, "(alice)")
	got := community.Write(d, at(7, 0))
	assert.Contains(t, got.Note, "Rollout 15 of payments")
	assert.NotContains(t, got.Note, "alice")
	assert.Equal(t, "alice", p.Attempt.Actor, "the incident keeps it")

	d.Action = incident.Resolve
	p.FixedBy = &change
	d.Incident = p
	assert.Contains(t,
		full.WriteResolvedBy(d, at(9, 0), change).Note, "alice")
	assert.NotContains(t,
		community.WriteResolvedBy(d, at(9, 0), change).Note, "alice")
}
