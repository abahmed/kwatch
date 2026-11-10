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
	require.Contains(t, pro.Note, "Fix: raise the memory limit")
	require.Contains(t, pro.Note, "kubectl")
	require.NotEmpty(t, pro.Steps)

	for name, w := range map[string]Writer{
		"community": community, "quiet": quiet,
	} {
		got := w.Write(d, at(5, 0))
		assert.NotContains(t, got.Note, "Fix:", name)
		assert.NotContains(t, got.Note, "kubectl", name)
		assert.Empty(t, got.Steps, name)
	}
	got := community.Write(d, at(5, 0))
	assert.Contains(t, noteFooter(got.Note), proFixWords)
	hidden := quiet.Write(d, at(5, 0))
	assert.NotContains(t, hidden.Note, proFixWords,
		"app.hideProHints removes the fix hint")
}

func TestCommunityProLineIsOneLineWithTheFixHint(t *testing.T) {
	_, community, _ := communityWriters()
	got := community.Write(announce(placedRollout()), at(5, 0))
	footer := noteFooter(got.Note)
	assert.Contains(t, footer, proFixWords)
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
	assert.NotContains(t, note, proFixWords)
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
	assert.Contains(t, got.Note, "It is already acknowledged")
}

// A close rival hides Pro's Fix line only until Pro's own checks settle
// it, so the Pro line still promises the fix. The Community message
// names neither cause nor the setting: the promise is all it carries.
func TestCommunityPromisesTheFixOfATiedCause(t *testing.T) {
	_, community, _ := communityWriters()
	p := placedRollout()
	cause := *p.Cause
	cause.RollbackRevision, cause.Change = "", nil // no rollback step
	cause.Rival = &rootcause.CauseRecord{Root: p.Root, Score: 0.94,
		Summary: "the rollout of checkout-rival"}
	p.Cause = &cause

	got := community.Write(announce(p), at(5, 0))

	assert.Contains(t, noteFooter(got.Note), proFixWords)
	for _, hidden := range []string{"Fix:", "256Mi", "memory limit",
		"two possible causes", "checkout-rival", "kubectl"} {
		assert.NotContains(t, got.Note, hidden)
	}
	assert.Empty(t, got.Steps)
}
