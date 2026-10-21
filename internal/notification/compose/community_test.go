package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
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
		"the one-line cause headline stays")
	assert.Less(t, len(got.Doc), len(pro.Doc))
	assert.NotContains(t, got.Note, "Checked:")
	assert.Equal(t, pro.Route, got.Route, "routing reads the incident")
	assert.NotEmpty(t, got.Steps, "the suggested steps stay")
}

func firstSentence(note string) string {
	first, _, _ := strings.Cut(note, ". ")
	return first
}

func TestCommunityNamesWhatProWouldAdd(t *testing.T) {
	p := solvedNodeOutage(t)
	p.Mode = detection.ModeNotReady
	_, community, quiet := communityWriters()
	d := announce(p)
	require.NotEmpty(t, d.Incident.Impact)

	got := community.Write(d, at(4, 0))
	hint := noteFooter(got.Note)
	assert.Equal(t, noteNode+proInvite, hint, "%q", got.Note)
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
	_, last, _ := strings.Cut(note, "\n")
	return last
}

func TestCommunityOmitsInvestigationOutput(t *testing.T) {
	d := announce(solvedNodeOutage(t))
	d.Facts.Output = []string{"panic: boom"}
	full, community, _ := communityWriters()

	assert.Equal(t, []string{"panic: boom"}, full.Write(d, at(4, 0)).Output)
	assert.Empty(t, community.Write(d, at(4, 0)).Output)
}
