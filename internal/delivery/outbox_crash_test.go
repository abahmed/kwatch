package delivery

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/storage"
)

// A crash between the provider accepting a message and the outbox
// forgetting it. The outbox is on the real state file; the removal never
// reaches the disk, as when the process is killed right after the send.
// The restart sends the message again, once, with the same conversation
// key, so the provider threads it, and a later restart sends nothing.
// kwatch does not mark the copy as a repeat: delivery is at least once.

// diskOutbox is the outbox over the state file, as the application
// keeps it.
type diskOutbox struct {
	values storage.Keyed[OutboxRecord]
	// dropRemovals makes removals vanish, as a crash before they were
	// written would; removed counts the ones asked for.
	dropRemovals bool
	mu           sync.Mutex
	removed      int
}

func (d *diskOutbox) LoadOutbox() ([]OutboxRecord, error) {
	var records []OutboxRecord
	err := d.values.Range("", func(_ string, r OutboxRecord) error {
		records = append(records, r)
		return nil
	})
	return records, err
}

func (d *diskOutbox) WriteOutbox(puts []OutboxRecord, removes []string) error {
	items := map[string]storage.Item[OutboxRecord]{}
	for _, r := range puts {
		items[r.ID] = storage.Item[OutboxRecord]{Value: r}
	}
	if err := d.values.PutAll(items); err != nil {
		return err
	}
	d.mu.Lock()
	d.removed += len(removes)
	d.mu.Unlock()
	if d.dropRemovals {
		return nil
	}
	return d.values.DeleteMany(removes)
}

func (d *diskOutbox) removals() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.removed
}

// session opens the state file, as a restart does, and builds a manager
// whose outbox is on it.
type session struct {
	store    *storage.Store
	outbox   *diskOutbox
	provider *scriptedProvider
	manager  *Manager
}

func openSession(t *testing.T, path string, crash bool) *session {
	t.Helper()
	store, err := storage.Open(path, storage.Options{Now: time.Now})
	require.NoError(t, err)
	_, err = store.Claim()
	require.NoError(t, err)
	s := &session{store: store, provider: newScriptedProvider(nil),
		outbox: &diskOutbox{
			values:       storage.OutboxValues[OutboxRecord](store),
			dropRemovals: crash,
		}}
	s.manager = outboxManager(t, s.provider, s.outbox)
	return s
}

// end stops the manager and closes the file: nothing is in flight after.
func (s *session) end(t *testing.T) {
	t.Helper()
	require.NoError(t, s.manager.Stop(context.Background()))
	require.NoError(t, s.store.Close())
}

func TestCrashBetweenSendAndOutboxRemovalSendsAgainOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	msg := notification.Message{Key: "deployment/shop/api",
		DedupKey: "alert-api-1", Revision: 2}

	first := openSession(t, path, true)
	// A crash can only lose a removal of a record that reached the
	// disk. The writer batches, and drops a record sent before it was
	// written, so each send waits until its record is on disk, as it
	// is when the queue is long enough for the writer to keep up.
	first.provider.send = func(_ context.Context, call int, _ string) error {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if r, err := first.outbox.LoadOutbox(); err == nil &&
				len(r) >= call {
				return nil
			}
			time.Sleep(time.Millisecond)
		}
		return errors.New("record never reached the disk")
	}
	require.NoError(t, first.manager.Start(context.Background()))
	first.manager.Notify("disk full")
	first.manager.NotifyIncident(msg)
	assert.ElementsMatch(t, []string{"disk full", msg.Key},
		[]string{first.provider.receive(t),
			first.provider.receive(t)})
	require.Eventually(t, func() bool { return first.outbox.removals() >= 2 },
		5*time.Second, time.Millisecond, "the writer asked to forget both")
	first.end(t) // The removals never reached the disk: the crash.

	second := openSession(t, path, false)
	require.NoError(t, second.manager.Start(context.Background()))
	assert.ElementsMatch(t, []string{"disk full", msg.Key},
		[]string{second.provider.receive(t),
			second.provider.receive(t)},
		"both are sent again, by the first restart")
	second.end(t)
	assert.Equal(t, 2, sentCalls(second.provider),
		"at most once more")

	third := openSession(t, path, false)
	require.NoError(t, third.manager.Start(context.Background()))
	third.end(t)
	assert.Zero(t, sentCalls(third.provider),
		"a second restart sends nothing")
	last := openSession(t, path, false)
	records, err := last.outbox.LoadOutbox()
	require.NoError(t, err)
	assert.Empty(t, records, "the outbox is empty")
	require.NoError(t, last.store.Close())
}
