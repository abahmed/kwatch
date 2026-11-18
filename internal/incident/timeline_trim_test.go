package incident

import (
	"encoding/json"
	"strconv"
	"strings"
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
// middle is thinned: the first entries stay, with the newest.
func TestTimelineKeepsUnreportedRepeats(t *testing.T) {
	inc := &Incident{}
	for i := 0; i < maxTimeline+5; i++ {
		inc.note(at(time.Duration(i)*time.Minute), "failing again")
	}

	require.Len(t, inc.Timeline, maxTimeline)
	assert.Equal(t, at(0), inc.Timeline[0].At, "how it began stays")
	assert.Equal(t, at(time.Duration(maxTimeline+4)*time.Minute),
		inc.Timeline[maxTimeline-1].At, "the newest stays")
	assert.Equal(t, 5, droppedIn(inc), "the dropped entries are counted")
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
	assert.Equal(t, "web-0", inc.Timeline[0].Entity.Name)
	assert.Equal(t, 10, droppedIn(inc))
}

// droppedIn counts the entries dropped from the middle of the timeline.
func droppedIn(inc *Incident) int {
	n := 0
	for _, e := range inc.Timeline {
		n += e.Gap
	}
	return n
}

// A long-lived incident with 500 updates keeps how it began (its first
// entries and every phase change) and where it stands (the newest),
// loses only member noise from the middle, and counts what it lost.
func TestLongLivedTimelineKeepsItsStartPhasesAndNewest(t *testing.T) {
	inc := &Incident{}
	inc.note(at(0), "kwatch noticed this")
	for i := 1; i <= 500; i++ {
		id := inventory.CoreID(kube.KindPod, "shop", "web-"+strconv.Itoa(i))
		at := at(time.Duration(i) * time.Minute)
		inc.noteAbout(at, "joined", &id)
		if i%100 == 0 {
			inc.note(at.Add(time.Second), "cause revised: step "+
				strconv.Itoa(i))
		}
	}

	require.Len(t, inc.Timeline, maxTimeline)
	assert.Equal(t, "kwatch noticed this", inc.Timeline[0].Text)
	for i := 0; i < timelineHead; i++ {
		assert.Equal(t, at(time.Duration(i)*time.Minute),
			inc.Timeline[i].At, "head entry %d", i)
	}
	var revised []string
	for _, e := range inc.Timeline {
		if strings.HasPrefix(e.Text, "cause revised") {
			revised = append(revised, e.Text)
		}
	}
	assert.Equal(t, []string{"cause revised: step 100",
		"cause revised: step 200", "cause revised: step 300",
		"cause revised: step 400", "cause revised: step 500"}, revised,
		"every phase change stays")
	assert.Equal(t, at(500*time.Minute).Add(time.Second),
		inc.Timeline[maxTimeline-1].At)
	assert.Equal(t, 506, maxTimeline+droppedIn(inc),
		"every entry is kept or counted as dropped")
	assert.Greater(t, inc.Timeline[timelineHead-1].Gap, 0,
		"the gap sits after the head")
}

// The gap counts survive the state file; a record saved before they
// existed loads with none.
func TestTimelineGapRoundTripsThroughARecord(t *testing.T) {
	inc := &Incident{}
	for i := 0; i < maxTimeline+7; i++ {
		inc.note(at(time.Duration(i)*time.Minute), "e")
	}
	data, err := json.Marshal(Record{Timeline: inc.Timeline})
	require.NoError(t, err)

	var back Record
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, 7, droppedIn(&Incident{Timeline: back.Timeline}))

	var old Record
	require.NoError(t, json.Unmarshal([]byte(
		`{"Timeline":[{"At":"2026-01-05T12:00:00Z","Text":"e"}]}`), &old))
	assert.Zero(t, old.Timeline[0].Gap)
}

// Entries that pile up while updates are held push the timeline past
// its cap, so its middle is thinned of entries no message covered yet.
// The reported count must still end right after the last line people
// got, or the next update would repeat lines already sent.
func TestTimelineDropOfUnsentEntriesKeepsTheReportedCount(t *testing.T) {
	inc := &Incident{}
	const sent, held = 30, 50
	for i := 0; i < sent; i++ {
		id := inventory.CoreID(kube.KindPod, "shop", "web-"+strconv.Itoa(i))
		inc.noteAbout(at(time.Duration(i)*time.Minute), "joined", &id)
		inc.sent.decided(true)
	}
	for i := 0; i < held; i++ {
		id := inventory.CoreID(kube.KindPod, "shop",
			"held-"+strconv.Itoa(i))
		inc.noteAbout(at(time.Duration(sent+i)*time.Minute), "joined",
			&id)
	}

	require.Len(t, inc.Timeline, maxTimeline)
	reported, known := inc.sent.reported(len(inc.Timeline))
	require.True(t, known)
	for i, e := range inc.Timeline {
		isSent := strings.HasPrefix(e.Entity.Name, "web-")
		assert.Equal(t, i < reported, isSent,
			"entry %d (%s) against reported %d", i, e.Entity.Name,
			reported)
	}
}
