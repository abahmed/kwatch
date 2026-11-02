package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxRetryDelayBacksOffToCap(t *testing.T) {
	box := newOutbox(newMemOutbox(), time.Now)
	want := []time.Duration{
		time.Second, time.Second, 2 * time.Second, 4 * time.Second,
		8 * time.Second, 16 * time.Second, outboxRetryMax, outboxRetryMax,
	}
	for i, delay := range want {
		box.failedWrites = max(i, 1)
		require.Equal(t, delay, box.retryDelay(), "failure %d", i)
	}
}

func TestOutboxRewritesFailedBatchWithoutNewChanges(t *testing.T) {
	store := &failingOutbox{memOutbox: newMemOutbox(), failures: 2}
	box := newOutbox(store, time.Now)
	armed := make(chan time.Duration, 4)
	fire := make(chan time.Time, 4)
	box.after = func(d time.Duration) <-chan time.Time {
		armed <- d
		return fire
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	box.start(ctx)
	defer func() { require.NoError(t, box.close(time.Second)) }()

	box.add(deliverJob{kind: jobMessage, msg: "m"}, "slack")
	require.Equal(t, outboxRetryBase, <-armed)
	fire <- time.Time{}
	require.Equal(t, 2*outboxRetryBase, <-armed, "second failure backs off")
	fire <- time.Time{}

	store.waitForRecords(t, 1)
}

// hookedOutbox runs during a write, then fails it.
type hookedOutbox struct {
	*memOutbox
	during func()
}

func (h *hookedOutbox) WriteOutbox([]OutboxRecord, []string) error {
	if h.during != nil {
		during := h.during
		h.during = nil
		during()
		return errors.New("disk unavailable")
	}
	return nil
}

// A record rewritten while its batch was failing keeps the newer send
// count: putting the failed batch back must not undo it.
func TestOutboxFailedBatchKeepsNewerRewrite(t *testing.T) {
	mem := newMemOutbox()
	store := &hookedOutbox{memOutbox: mem}
	box := newOutbox(store, time.Now)
	job := deliverJob{kind: jobMessage, msg: "m"}
	job.outboxID = box.add(job, "slack")
	box.setAttempts(job, "slack", 1)
	store.during = func() { box.setAttempts(job, "slack", 0) }

	require.False(t, box.flush())

	box.mu.Lock()
	defer box.mu.Unlock()
	require.Contains(t, box.puts, job.outboxID)
	assert.Zero(t, box.puts[job.outboxID].Attempts,
		"the newer rewrite is kept")
}
