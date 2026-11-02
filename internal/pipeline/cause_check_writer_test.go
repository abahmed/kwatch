package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/storage"
)

func newCountsWriter(store HistoryStore) *historyWriter {
	return newHistoryWriter(store, inventory.NewModel(inventory.Options{}),
		wallTimer, &workerStats{})
}

func TestHistoryWriterChecksWriteOnceThenNothingToFlush(t *testing.T) {
	st := &countsStore{}
	w := newCountsWriter(st)

	w.addChecks(json.RawMessage(`{"n":1}`))
	require.Len(t, w.wake, 1, "adding a value wakes the writer")
	assert.True(t, w.flushChecks())
	assert.True(t, w.flushChecks(), "nothing queued is not a failure")

	assert.Equal(t, []string{`{"n":1}`}, st.checkSaves)
	assert.JSONEq(t, `{"n":1}`, string(st.checks))
}

func TestHistoryWriterChecksKeepOnlyTheNewestValue(t *testing.T) {
	st := &countsStore{}
	w := newCountsWriter(st)

	w.addChecks(json.RawMessage(`{"n":1}`))
	w.addChecks(json.RawMessage(`{"n":2}`))
	assert.True(t, w.flushChecks())

	assert.Equal(t, []string{`{"n":2}`}, st.checkSaves)
}

func TestHistoryWriterChecksFailedWriteIsKeptAndRetried(t *testing.T) {
	st := &countsStore{err: errCountsStore}
	w := newCountsWriter(st)

	w.addChecks(json.RawMessage(`{"n":1}`))
	assert.False(t, w.flushChecks())
	assert.Nil(t, st.checks)

	st.setErr(nil)
	assert.True(t, w.flushChecks())
	assert.JSONEq(t, `{"n":1}`, string(st.checks))
	assert.Len(t, st.checkSaves, 2)
}

func TestHistoryWriterChecksNewerValueBeatsAFailedOne(t *testing.T) {
	st := &countsStore{err: errCountsStore}
	w := newCountsWriter(st)

	w.addChecks(json.RawMessage(`{"n":1}`))
	assert.False(t, w.flushChecks())
	w.addChecks(json.RawMessage(`{"n":2}`))
	st.setErr(nil)
	assert.True(t, w.flushChecks())

	assert.JSONEq(t, `{"n":2}`, string(st.checks))
}

func TestHistoryWriterChecksFencedStoreStopsTheWriter(t *testing.T) {
	st := &countsStore{err: storage.ErrFenced}
	w := newCountsWriter(st)

	w.addChecks(json.RawMessage(`{"n":1}`))

	assert.False(t, w.flushChecks())
	assert.True(t, w.fenced.Load())
}

func TestHistoryWriterChecksDropsValueWithoutAKeepingStore(t *testing.T) {
	w := newCountsWriter(&failingHistory{})

	w.addChecks(json.RawMessage(`{"n":1}`))

	assert.True(t, w.flushChecks())
	assert.Nil(t, w.checks, "a store that cannot keep it forgets it")
}

func TestHistoryWriterFlushWritesChecks(t *testing.T) {
	st := &countsStore{}
	w := newCountsWriter(st)

	w.addChecks(json.RawMessage(`{"n":3}`))
	w.flush()

	assert.JSONEq(t, `{"n":3}`, string(st.checks))
}

func TestHistoryRecorderSaveChecksNeedsAWriter(t *testing.T) {
	assert.NotPanics(t, func() {
		(&historyRecorder{}).saveChecks(json.RawMessage(`{}`))
	})

	w := newCountsWriter(&countsStore{})
	(&historyRecorder{writer: w}).saveChecks(json.RawMessage(`{"n":4}`))
	assert.JSONEq(t, `{"n":4}`, string(w.checks))
}

func TestHistoryWriterChecksRoundTripThroughTheDiskStore(t *testing.T) {
	disk := NewIncidentStore(openTempStore(t))
	w := newCountsWriter(disk.(HistoryStore))

	w.addChecks(json.RawMessage(`{"n":5}`))
	require.True(t, w.flushChecks())

	raw, ok, err := disk.(causeStore).LoadCauseChecks()
	require.NoError(t, err)
	require.True(t, ok)
	assert.JSONEq(t, `{"n":5}`, string(raw))
}
