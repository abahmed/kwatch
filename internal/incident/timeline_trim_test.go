package incident

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func noteTimes(inc *Incident, from, n int, text string) {
	for i := 0; i < n; i++ {
		inc.note(at(time.Duration(from+i)*time.Minute), text)
		inc.sent.decided(true)
	}
}

func totalNotes(inc *Incident) int {
	n := 0
	for _, e := range inc.Timeline {
		n += 1 + e.Repeats
	}
	return n
}

// A failure that keeps coming back folds into one entry with a count:
// the start of the story stays, and no occurrence goes uncounted.
func TestTimelineFoldsRepeatsBeforeDroppingHistory(t *testing.T) {
	inc := &Incident{}
	noteTimes(inc, 0, 1, "kwatch noticed this")
	noteTimes(inc, 1, 70, "failing again")

	require.LessOrEqual(t, len(inc.Timeline), maxTimeline)
	assert.Equal(t, "kwatch noticed this", inc.Timeline[0].Text,
		"the first entry survives the repeats")
	assert.Equal(t, 71, totalNotes(inc), "every note is counted")
	reported, known := inc.sent.reported(len(inc.Timeline))
	assert.True(t, known)
	assert.Equal(t, len(inc.Timeline), reported,
		"the delivered mark still covers the whole timeline")
}

// Entries no delivered message covered are never folded: the next
// update must still report each of them. Without a repeat to fold the
// oldest entries go, as before.
func TestTimelineKeepsUnreportedRepeats(t *testing.T) {
	inc := &Incident{}
	for i := 0; i < maxTimeline+5; i++ {
		inc.note(at(time.Duration(i)*time.Minute), "failing again")
	}

	require.Len(t, inc.Timeline, maxTimeline)
	assert.Equal(t, at(5*time.Minute), inc.Timeline[0].At,
		"nothing was reported, so the oldest entries go")
	for _, e := range inc.Timeline {
		assert.Zero(t, e.Repeats)
	}
}

// Entries about different members, or with different text, are not
// repeats of each other.
func TestTimelineFoldsOnlyTheSameEventAboutTheSameMember(t *testing.T) {
	inc := &Incident{}
	for i := 0; i < maxTimeline+10; i++ {
		id := inventory.CoreID(kube.KindPod, "shop", "web-"+strconv.Itoa(i))
		inc.noteAbout(at(time.Duration(i)*time.Minute), "joined", &id)
		inc.sent.decided(true)
	}

	require.Len(t, inc.Timeline, maxTimeline)
	for _, e := range inc.Timeline {
		assert.Zero(t, e.Repeats, "distinct members are kept apart")
	}
	assert.Equal(t, "web-10", inc.Timeline[0].Entity.Name)
}
