package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdRig fills the timeline with two objects, 400 entries each, one
// every ten minutes over almost three days: "open" belongs to an issue
// still open and is held, with a mark at its entry 200; "closed" is not.
func holdRig(t *testing.T) (*Store, Log[string], time.Time) {
	t.Helper()
	clock := newFakeClock()
	start := clock.Now().Add(-72 * time.Hour)
	s := openClaimed(t, clock)
	log := TimelineLog[string](s)
	var entries []Entry[string]
	for i := range 400 {
		at := start.Add(time.Duration(i) * 10 * time.Minute)
		for _, entity := range []string{"open", "closed"} {
			entries = append(entries, Entry[string]{Entity: entity,
				At: at, Value: fmt.Sprintf("%03d", i)})
		}
	}
	require.NoError(t, log.AppendAll(entries))
	mark := start.Add(200 * 10 * time.Minute)
	require.NoError(t, log.Hold(map[string]LogHold{
		"open": {Marks: []time.Time{mark}}}))
	return s, log, mark
}

// A full timeline evicts the detail of objects nobody holds first, then
// thins the middle of a held one, and keeps its beginning, its marks
// and its newest entries.
func TestTimelineCapKeepsTheBeginningOfOpenIssues(t *testing.T) {
	s, log, mark := holdRig(t)
	size := entrySize(t, "open", "000")
	policy := Policy{TimelineCap: 300 * size, Batch: 50}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 500, result.Evicted)
	assert.Empty(t, collect(t, log, "closed", time.Time{}, time.Time{}),
		"the detail nobody holds goes first")
	open := collect(t, log, "open", time.Time{}, time.Time{})
	require.Len(t, open, 300)
	assert.Equal(t, []string{"000", "001", "002", "003", "004", "005",
		"006", "007"}, open[:holdHead], "its beginning is kept")
	assert.Equal(t, "399", open[len(open)-1])
	assert.Equal(t, "368", open[len(open)-holdTail], "its newest kept")
	assert.Len(t, collect(t, log, "open", mark, mark.Add(time.Second)), 1,
		"its mark is kept")
	assert.Contains(t, open, "012", "each old hour keeps one entry")
	assert.Contains(t, open, "018")
}

// Past the retention the marked entries of a held object stay; the
// rest expires as before.
func TestTimelineRetentionKeepsMarksOfOpenIssues(t *testing.T) {
	s, log, mark := holdRig(t)
	policy := Policy{Batch: 50,
		Retention: map[Bucket]time.Duration{Timeline: time.Hour}}

	_, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Empty(t, collect(t, log, "closed", time.Time{}, time.Time{}))
	assert.Equal(t, []string{"200"},
		collect(t, log, "open", time.Time{}, time.Time{}), "the mark")
	assert.Len(t, collect(t, log, "open", mark, mark.Add(time.Second)), 1)
}
