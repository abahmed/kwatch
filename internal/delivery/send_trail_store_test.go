package delivery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

// trailOutbox is a memOutbox that also keeps the send trail. fail makes
// the next trail writes fail.
type trailOutbox struct {
	*memOutbox
	tmu    sync.Mutex
	trails map[string]TrailRecord
	fail   bool
	wrote  chan struct{}
}

func newTrailOutbox(records ...TrailRecord) *trailOutbox {
	s := &trailOutbox{memOutbox: newMemOutbox(),
		trails: map[string]TrailRecord{}, wrote: make(chan struct{}, 64)}
	for _, r := range records {
		s.trails[r.Key] = r
	}
	return s
}

func (s *trailOutbox) LoadTrails() ([]TrailRecord, error) {
	s.tmu.Lock()
	defer s.tmu.Unlock()
	out := make([]TrailRecord, 0, len(s.trails))
	for _, r := range s.trails {
		out = append(out, r)
	}
	return out, nil
}

func (s *trailOutbox) WriteTrails(
	puts []TrailRecord, removes []string,
) error {
	s.tmu.Lock()
	if s.fail {
		s.tmu.Unlock()
		return errors.New("disk full")
	}
	for _, r := range puts {
		s.trails[r.Key] = r
	}
	for _, key := range removes {
		delete(s.trails, key)
	}
	s.tmu.Unlock()
	select {
	case s.wrote <- struct{}{}:
	default:
	}
	return nil
}

func (s *trailOutbox) trail(key string) []SendAttempt {
	s.tmu.Lock()
	defer s.tmu.Unlock()
	return s.trails[key].Attempts
}

func (s *trailOutbox) keys() int {
	s.tmu.Lock()
	defer s.tmu.Unlock()
	return len(s.trails)
}

func savedTrail(key string, at time.Time, results ...string) TrailRecord {
	r := TrailRecord{Version: trailRecordVersion, Key: key}
	for i, result := range results {
		r.Attempts = append(r.Attempts, SendAttempt{
			At: at.Add(time.Duration(i) * time.Second), Result: result})
	}
	return r
}

// A trail saved by one session is what the next session shows, and its
// new attempts are added after the saved ones.
func TestSendTrailSurvivesRestart(t *testing.T) {
	store := newTrailOutbox()
	first := outboxManager(t, newScriptedProvider(nil), store)
	require.NoError(t, first.Start(context.Background()))
	first.NotifyIncident(notification.Message{Key: "inc-1",
		Carrier: "digest"})
	require.NoError(t, first.Stop(context.Background()))
	require.Len(t, store.trail("inc-1"), 1)

	second := outboxManager(t, newScriptedProvider(nil), store)
	got := second.SendTrail("inc-1")
	require.Len(t, got, 1)
	assert.Equal(t, "carried", got[0].Result)
	assert.Equal(t, "digest", got[0].Carrier)

	require.NoError(t, second.Start(context.Background()))
	second.NotifyIncident(notification.Message{Key: "inc-1",
		Carrier: "dropped"})
	require.NoError(t, second.Stop(context.Background()))
	saved := store.trail("inc-1")
	require.Len(t, saved, 2)
	assert.Equal(t, "dropped", saved[1].Carrier)
}

// Saved attempts go before the ones this session noted before the
// store was attached, and the merged trail keeps its bound.
func TestSendTrailRestoreMergesOlderAttemptsFirst(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	saved := make([]string, maxTrailAttempts)
	for i := range saved {
		saved[i] = "ok"
	}
	var trail sendTrail
	trail.add("inc-1", SendAttempt{At: at.Add(time.Hour),
		Result: "retrying"})
	trail.restore([]TrailRecord{savedTrail("inc-1", at, saved...)})
	got := trail.of("inc-1")
	require.Len(t, got, maxTrailAttempts)
	assert.Equal(t, "retrying", got[len(got)-1].Result)
	puts, _ := trail.take()
	require.Len(t, puts, 1, "the merged trail is written again")
}

// Restoring more incidents than the bound forgets the oldest and
// deletes them from the store; a record of another layout is deleted.
func TestSendTrailRestoreIsBounded(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	var records []TrailRecord
	for i := range maxTrailKeys + 2 {
		records = append(records, savedTrail(fmt.Sprint("inc-", i),
			at.Add(time.Duration(i)*time.Minute), "ok"))
	}
	stale := savedTrail("old-layout", at, "ok")
	stale.Version = trailRecordVersion + 1
	records = append(records, stale)

	var trail sendTrail
	trail.restore(records)
	assert.Nil(t, trail.of("inc-0"))
	assert.Nil(t, trail.of("inc-1"))
	assert.NotNil(t, trail.of("inc-2"))
	assert.Nil(t, trail.of("old-layout"))
	assert.Len(t, trail.byKey, maxTrailKeys)
	puts, removes := trail.take()
	assert.Empty(t, puts)
	assert.ElementsMatch(t, []string{"inc-0", "inc-1", "old-layout"},
		removes)
}

// A failed trail write is retried by a later flush; an incident the
// bound forgot is deleted from the store.
func TestSendTrailFailedWriteIsRetried(t *testing.T) {
	store := newTrailOutbox()
	store.fail = true
	box := newOutbox(store, time.Now)
	var trail sendTrail
	box.trails, box.trail = store, &trail
	trail.restore(nil)
	trail.add("inc-1", SendAttempt{Result: "ok"})

	ok, wrote := box.flushBatch()
	assert.False(t, ok)
	assert.True(t, wrote)
	assert.Zero(t, store.keys())

	store.tmu.Lock()
	store.fail = false
	store.tmu.Unlock()
	ok, _ = box.flushBatch()
	require.True(t, ok)
	assert.Len(t, store.trail("inc-1"), 1)

	for i := range maxTrailKeys {
		trail.add(fmt.Sprint("k", i), SendAttempt{Result: "ok"})
	}
	ok, _ = box.flushBatch()
	require.True(t, ok)
	assert.Nil(t, store.trail("inc-1"))
	assert.Equal(t, maxTrailKeys, store.keys())
}

// A store without a trail leaves the trail in memory only.
func TestSendTrailWithoutTrailStoreStaysInMemory(t *testing.T) {
	store := newMemOutbox()
	manager := outboxManager(t, newScriptedProvider(nil), store)
	manager.NotifyIncident(notification.Message{Key: "inc-1",
		Carrier: "digest"})
	assert.Len(t, manager.SendTrail("inc-1"), 1)
	assert.Nil(t, manager.outbox.Load().trail)
}

// loadHookOutbox runs onLoad before the trail is loaded, the window in
// which another attach can win.
type loadHookOutbox struct {
	*trailOutbox
	onLoad func()
}

func (s *loadHookOutbox) LoadTrails() ([]TrailRecord, error) {
	if hook := s.onLoad; hook != nil {
		s.onLoad = nil
		hook()
	}
	return s.trailOutbox.LoadTrails()
}

// An attach that loses the race restores nothing: the saved trail is
// restored once, and the winning outbox keeps the trail writer.
func TestSendTrailAttachRestoresOnce(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	saved := newTrailOutbox(savedTrail("inc-1", at, "ok"))
	manager := outboxManager(t, newScriptedProvider(nil), nil)
	winner := &loadHookOutbox{trailOutbox: saved}
	loser := &loadHookOutbox{trailOutbox: saved, onLoad: func() {
		require.NoError(t, manager.AttachOutbox(winner))
	}}
	require.Error(t, manager.AttachOutbox(loser))
	assert.Len(t, manager.SendTrail("inc-1"), 1)
	box := manager.outbox.Load()
	require.NotNil(t, box)
	assert.Same(t, winner, box.trails)
}

// Records restore drops are deleted by a write that is woken at once,
// not at the next send.
func TestSendTrailRestoreWakesTheWriterForDeletions(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	stale := savedTrail("old-layout", at, "ok")
	stale.Version = trailRecordVersion + 1
	var trail sendTrail
	wakes := 0
	trail.onChange(func() { wakes++ })

	trail.restore([]TrailRecord{stale})

	assert.Equal(t, 1, wakes)
	_, removes := trail.take()
	assert.Equal(t, []string{"old-layout"}, removes)
}

// A writer attached after restore is woken for the waiting deletions.
func TestSendTrailOnChangeWakesForWaitingDeletions(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	stale := savedTrail("old-layout", at, "ok")
	stale.Version = trailRecordVersion + 1
	var trail sendTrail
	trail.restore([]TrailRecord{stale})
	wakes := 0

	trail.onChange(func() { wakes++ })

	assert.Equal(t, 1, wakes)
}

// Without a trail store nothing ever takes the changes, so none are
// noted: the trail stays bounded and so does what waits to be written.
func TestSendTrailWithoutTrailStoreNotesNoChanges(t *testing.T) {
	manager := outboxManager(t, newScriptedProvider(nil), newMemOutbox())
	for i := range 20000 {
		manager.NotifyIncident(notification.Message{
			Key: fmt.Sprint("inc-", i), Carrier: "digest"})
	}
	manager.trail.mu.Lock()
	defer manager.trail.mu.Unlock()
	assert.Empty(t, manager.trail.dirty)
	assert.Len(t, manager.trail.byKey, maxTrailKeys)
}

// Incidents told before the trail store is attached are written once it
// is, as are the changes after it.
func TestSendTrailAttachWritesWhatWasToldBefore(t *testing.T) {
	var trail sendTrail
	for i := range 3 {
		trail.add(fmt.Sprint("inc-", i), SendAttempt{})
	}
	trail.mu.Lock()
	require.Empty(t, trail.dirty, "no store yet")
	trail.mu.Unlock()

	trail.restore(nil)
	trail.add("inc-3", SendAttempt{})
	puts, removes := trail.take()

	assert.Len(t, puts, 4)
	assert.Empty(t, removes)
}
