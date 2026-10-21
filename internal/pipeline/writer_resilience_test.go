package pipeline

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/storage"
)

// captureLog returns what klog writes until the test ends.
func captureLog(t *testing.T) func() string {
	t.Helper()
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})
	return func() string {
		klog.Flush()
		return out.String()
	}
}

// lines counts the distinct log lines containing text: klog repeats a
// line once per severity it reaches when one buffer takes them all.
func lines(log, text string) int {
	seen := map[string]bool{}
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, text) {
			seen[line] = true
		}
	}
	return len(seen)
}

func TestStoreWriterLogsAStoreThatStaysDownRarely(t *testing.T) {
	logged := captureLog(t)
	st := &memStore{saveErr: errStore}
	w := newStoreWriter(st, wallTimer, &workerStats{})

	for i := 0; i < 70; i++ {
		w.offer(records("a"))
		w.flush()
	}

	// The first failing batch, then every writeLogEvery-th one.
	assert.Equal(t, 3, lines(logged(), "storage write failed"))

	st.mu.Lock()
	st.saveErr = nil
	st.mu.Unlock()
	w.flush()
	assert.Equal(t, 1, lines(logged(), "storage writes recovered"))
}

// panickingStore panics in SaveIncidents while panics is above zero.
type panickingStore struct {
	*memStore
	panics int
}

func (p *panickingStore) SaveIncidents(r []incident.Record) error {
	if p.panics > 0 {
		p.panics--
		panic("store blew up")
	}
	return p.memStore.SaveIncidents(r)
}

func TestStoreWriterRetriesABatchAfterAStorePanic(t *testing.T) {
	st := &panickingStore{memStore: &memStore{}, panics: 1}
	w := newStoreWriter(st, wallTimer, &workerStats{})
	w.offer(records("a"))

	w.flush() // the store panics: contained, the batch is kept
	w.flush() // the retry succeeds

	saved, _ := st.counts()
	assert.Equal(t, 1, saved, "only the retry reached the store")
	require.Len(t, st.latest(), 1)
	assert.Equal(t, "a", st.latest()[0].ID)
	assert.Equal(t, int64(1), w.stats.writeFailures.Load())
}

// scriptedHistory is a history store whose AppendTimeline returns err, or
// panics while panics is above zero.
type scriptedHistory struct {
	failingHistory
	err    error
	panics int
}

func (s *scriptedHistory) AppendTimeline(entries []TimelineEntry) error {
	if s.panics > 0 {
		s.panics--
		panic("store blew up")
	}
	if s.err != nil {
		return s.err
	}
	return s.failingHistory.AppendTimeline(entries)
}

func oneChange(i int) TimelineEntry {
	deployment := inventory.CoreID("deployment", "shop", "api")
	return changeEntry(inventory.Change{
		Entity: deployment, At: timelineAt(time.Duration(i) * time.Second),
		Fields: []inventory.FieldChange{{Path: "spec.template"}},
	})
}

func readyTimer(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func TestHistoryWriterStopsForGoodWhenFenced(t *testing.T) {
	store := &scriptedHistory{
		err: fmt.Errorf("append: %w", storage.ErrFenced)}
	w := newHistoryWriter(store, inventory.NewModel(inventory.Options{}),
		readyTimer, &workerStats{})
	go w.run()
	w.add(oneChange(0))

	select {
	case <-w.done:
	case <-time.After(5 * time.Second):
		t.Fatal("a fenced history writer must stop on its own")
	}

	w.add(oneChange(1))
	w.mu.Lock()
	defer w.mu.Unlock()
	assert.Empty(t, w.pending, "a fenced writer keeps nothing pending")
}

func TestHistoryWriterSurvivesAStorePanic(t *testing.T) {
	store := &scriptedHistory{panics: 1}
	w := newHistoryWriter(store, inventory.NewModel(inventory.Options{}),
		readyTimer, &workerStats{})

	w.add(oneChange(0))
	w.guardedFlush() // the store panics: contained, that batch is lost
	w.add(oneChange(1))
	w.guardedFlush()

	assert.Len(t, store.timeline, 1)
	assert.Equal(t, int64(1), w.stats.writeFailures.Load())
}
