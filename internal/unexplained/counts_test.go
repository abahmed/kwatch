package unexplained

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var day0 = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func TestCountsSumPerModeAndReason(t *testing.T) {
	c := New()
	c.Record(KindEvent, "UnusualEvent", "Odd", "first", 2, day0)
	c.Record(KindEvent, "UnusualEvent", "Odd", "second", 3, day0)
	c.Record(KindEvent, "UnusualEvent", "Rare", "x", 1, day0)
	c.RecordIncident("i1", "CrashLoop", "CrashLoopBackOff", "boom", day0)

	got := c.Summary(day0, 10)

	assert.Equal(t, 6, got.Events)
	assert.Equal(t, 1, got.Incidents)
	require.Len(t, got.Top, 3)
	assert.Equal(t, "Odd", got.Top[0].Reason)
	assert.Equal(t, 5, got.Top[0].Count)
	assert.Equal(t, []string{"first", "second"}, got.Top[0].Samples)
}

func TestAnIncidentIsCountedOnce(t *testing.T) {
	c := New()
	assert.True(t, c.RecordIncident("i1", "M", "R", "", day0))
	assert.False(t, c.RecordIncident("i1", "M", "R", "", day0))
	assert.Equal(t, 1, c.Summary(day0, 5).Incidents)
}

func TestOldDaysFallOutOfTheWindow(t *testing.T) {
	c := New()
	c.Record(KindEvent, "M", "Old", "", 4, day0)
	later := day0.AddDate(0, 0, Window)
	c.Record(KindEvent, "M", "New", "", 1, later)

	got := c.Summary(later, 5)

	assert.Equal(t, 1, got.Events)
	require.Len(t, got.Top, 1)
	assert.Equal(t, "New", got.Top[0].Reason)
}

func TestRowsAndSamplesAreBounded(t *testing.T) {
	c := New()
	for i := range 3 * MaxRows {
		c.Record(KindEvent, "M", "Reason"+strconv.Itoa(i), "s", 1, day0)
	}
	long := strings.Repeat("x", 10*MaxSampleRunes)
	for i := range 10 {
		c.Record(KindEvent, "M", "Reason0", long+strconv.Itoa(i), 1, day0)
	}

	got := c.Summary(day0, 1000)

	assert.LessOrEqual(t, len(got.Top), MaxRows+1)
	assert.Equal(t, 3*MaxRows+10, got.Events)
	for _, row := range got.Top {
		assert.LessOrEqual(t, len(row.Samples), MaxSamples)
		for _, sample := range row.Samples {
			assert.LessOrEqual(t, len([]rune(sample)), MaxSampleRunes)
		}
	}
}

func TestSamplesAreRedacted(t *testing.T) {
	c := New()
	c.Record(KindEvent, "M", "R",
		"failed with password=hunter2hunter2 for the app", 1, day0)

	sample := c.Summary(day0, 5).Top[0].Samples[0]

	assert.NotContains(t, sample, "hunter2")
}

func TestCountsSurviveARestart(t *testing.T) {
	c := New()
	c.Record(KindEvent, "M", "R", "s", 2, day0)
	c.RecordIncident("i1", "Mode", "Reason", "e", day0)
	raw, err := json.Marshal(c)
	require.NoError(t, err)

	restored := New()
	require.NoError(t, json.Unmarshal(raw, restored))

	assert.Equal(t, c.Summary(day0, 5), restored.Summary(day0, 5))
	assert.Equal(t, day0, restored.Latest())
	assert.False(t, restored.RecordIncident("i1", "Mode", "Reason", "", day0))
}

func TestADamagedValueCannotGrowTheCounts(t *testing.T) {
	var in wire
	for i := range 5 * MaxRows {
		in.Rows = append(in.Rows, wireRow{Kind: KindEvent, Mode: "M",
			Reason: strconv.Itoa(i), Days: map[string]int{
				"2026-03-10": 1, "not a day": 5}})
	}
	raw, err := json.Marshal(in)
	require.NoError(t, err)

	c := New()
	require.NoError(t, json.Unmarshal(raw, c))

	assert.LessOrEqual(t, len(c.Summary(day0, 1000).Top), MaxRows)
	assert.Equal(t, MaxRows, c.Summary(day0, 1000).Events)
}

func TestUnknownKindsAndEmptyCountsAreIgnored(t *testing.T) {
	c := New()
	c.Record("other", "M", "R", "", 1, day0)
	c.Record(KindEvent, "M", "R", "", 0, day0)
	assert.Zero(t, c.Summary(day0, 5).Events)
}

// The overflow rows count inside MaxRows: what is counted before a
// restart is counted after it.
func TestOverflowSurvivesARestore(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	c := New()
	for i := range MaxRows + 5 {
		c.Record(KindEvent, "mode", "reason-"+strconv.Itoa(i), "", 1, now)
		c.Record(KindIncident, "mode", "cause-"+strconv.Itoa(i), "", 1, now)
	}
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	d := New()
	require.NoError(t, json.Unmarshal(raw, d))
	assert.Equal(t, c.Summary(now, 0), d.Summary(now, 0))
}

// Latest follows the events counted only: an incident counted later
// does not mark events before it as read.
func TestLatestIgnoresIncidents(t *testing.T) {
	t0 := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	c := New()
	c.Record(KindEvent, "mode", "reason", "", 1, t0)
	c.RecordIncident("inc-1", "mode", "cause", "", t0.Add(time.Minute))
	assert.Equal(t, t0, c.Latest())
}
