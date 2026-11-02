package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// placedRollout is the bad rollout with its cause in a setting, so Pro
// names the place to change.
func placedRollout() incident.Incident {
	p := badRollout()
	cause := *p.Cause
	cause.Place = place("app", "resources.limits.memory", "256Mi")
	p.Cause = &cause
	return p
}

func TestCommunityLeavesTheFixToPro(t *testing.T) {
	full, community, quiet := communityWriters()
	d := announce(placedRollout())

	pro := full.Write(d, at(5, 0))
	require.Contains(t, pro.Note, "Fix in: deployment/payments")
	require.Contains(t, pro.Note, "kubectl")
	require.NotEmpty(t, pro.Steps)

	for name, w := range map[string]Writer{
		"community": community, "quiet": quiet,
	} {
		got := w.Write(d, at(5, 0))
		assert.NotContains(t, got.Note, "Fix in:", name)
		assert.NotContains(t, got.Note, "kubectl", name)
		assert.Empty(t, got.Steps, name)
	}
	got := community.Write(d, at(5, 0))
	assert.Contains(t, noteFooter(got.Note), proFixTail)
	hidden := quiet.Write(d, at(5, 0))
	assert.NotContains(t, hidden.Note, proFixTail,
		"app.hideProHints removes the fix hint")
}

func TestCommunityProLineIsOneLineWithTheFixHint(t *testing.T) {
	_, community, _ := communityWriters()
	got := community.Write(announce(placedRollout()), at(5, 0))
	footer := noteFooter(got.Note)
	assert.Contains(t, footer, proFixTail)
	assert.Equal(t, 1, strings.Count(got.Note, "kwatch.dev/pro"))
	assert.Equal(t, 1, strings.Count(got.Note, "Pro shows"))
}

func TestCommunityHasNoFixHintWithoutAFix(t *testing.T) {
	_, community, _ := communityWriters()
	d := announce(badRollout())
	d.Incident.Cause = &rootcause.CauseRecord{Summary: "it broke"}
	d.Incident.Members = nil
	note := community.Write(d, at(5, 0)).Note
	require.NotEmpty(t, note)
	assert.NotContains(t, note, proFixTail)
}

func TestCommunityLeavesRecurrenceToPro(t *testing.T) {
	full, community, _ := communityWriters()
	p := badRollout()
	p.History = []incident.Occurrence{
		{Opened: at(1, 0), Heard: true},
		{Opened: at(1, 30), Heard: true},
	}
	d := announce(p)

	require.Contains(t, full.Write(d, at(5, 0)).Note,
		"third time this week")
	got := community.Write(d, at(5, 0))
	assert.NotContains(t, got.Note, "time this week")
}

func TestCommunityKeepsTheAckNotice(t *testing.T) {
	_, community, _ := communityWriters()
	got := community.Write(announce(ackedPod("", true)), writerNow)
	assert.Contains(t, got.Note, "The ack annotation is still present")
}
