package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/storage"
)

func TestHistoryWriterUnexplainedWriteOnceThenNothingToFlush(t *testing.T) {
	st := &countsStore{}
	w := newCountsWriter(st)

	w.addUnexplained(json.RawMessage(`{"n":1}`))
	require.Len(t, w.wake, 1, "adding a value wakes the writer")
	assert.True(t, w.flushUnexplained())
	assert.True(t, w.flushUnexplained(), "nothing queued is not a failure")

	assert.Equal(t, []string{`{"n":1}`}, st.unexplainedSaves)
	assert.JSONEq(t, `{"n":1}`, string(st.unexplained))
}

func TestHistoryWriterUnexplainedKeepOnlyTheNewestValue(t *testing.T) {
	st := &countsStore{}
	w := newCountsWriter(st)

	w.addUnexplained(json.RawMessage(`{"n":1}`))
	w.addUnexplained(json.RawMessage(`{"n":2}`))
	assert.True(t, w.flushUnexplained())

	assert.Equal(t, []string{`{"n":2}`}, st.unexplainedSaves)
}

func TestHistoryWriterUnexplainedFailedWriteIsKeptAndRetried(t *testing.T) {
	st := &countsStore{err: errCountsStore}
	w := newCountsWriter(st)

	w.addUnexplained(json.RawMessage(`{"n":1}`))
	assert.False(t, w.flushUnexplained())
	assert.Nil(t, st.unexplained)

	st.setErr(nil)
	assert.True(t, w.flushUnexplained())
	assert.JSONEq(t, `{"n":1}`, string(st.unexplained))
	assert.Len(t, st.unexplainedSaves, 2)
}

func TestHistoryWriterUnexplainedNewerValueBeatsAFailedOne(t *testing.T) {
	st := &countsStore{err: errCountsStore}
	w := newCountsWriter(st)

	w.addUnexplained(json.RawMessage(`{"n":1}`))
	assert.False(t, w.flushUnexplained())
	w.addUnexplained(json.RawMessage(`{"n":2}`))
	st.setErr(nil)
	assert.True(t, w.flushUnexplained())

	assert.JSONEq(t, `{"n":2}`, string(st.unexplained))
}

func TestHistoryWriterUnexplainedFencedStoreStopsTheWriter(t *testing.T) {
	st := &countsStore{err: storage.ErrFenced}
	w := newCountsWriter(st)

	w.addUnexplained(json.RawMessage(`{"n":1}`))

	assert.False(t, w.flushUnexplained())
	assert.True(t, w.fenced.Load())
}

func TestHistoryWriterUnexplainedDropsValueWithoutAKeepingStore(t *testing.T) {
	w := newCountsWriter(&failingHistory{})

	w.addUnexplained(json.RawMessage(`{"n":1}`))

	assert.True(t, w.flushUnexplained())
	assert.Nil(t, w.unexplained, "a store that cannot keep it forgets it")
}

func TestHistoryWriterFlushWritesUnexplained(t *testing.T) {
	st := &countsStore{}
	w := newCountsWriter(st)

	w.addUnexplained(json.RawMessage(`{"n":3}`))
	w.flush()

	assert.JSONEq(t, `{"n":3}`, string(st.unexplained))
}

func TestHistoryRecorderSaveUnexplainedNeedsAWriter(t *testing.T) {
	assert.NotPanics(t, func() {
		(&historyRecorder{}).saveUnexplained(json.RawMessage(`{}`))
	})

	w := newCountsWriter(&countsStore{})
	(&historyRecorder{writer: w}).saveUnexplained(json.RawMessage(`{"n":4}`))
	assert.JSONEq(t, `{"n":4}`, string(w.unexplained))
}

func TestHistoryWriterUnexplainedRoundTripThroughTheDiskStore(t *testing.T) {
	disk := NewIncidentStore(openTempStore(t))
	w := newCountsWriter(disk.(HistoryStore))

	w.addUnexplained(json.RawMessage(`{"n":5}`))
	require.True(t, w.flushUnexplained())

	raw, ok, err := disk.(unexplainedStore).LoadUnexplained()
	require.NoError(t, err)
	require.True(t, ok)
	assert.JSONEq(t, `{"n":5}`, string(raw))
}
