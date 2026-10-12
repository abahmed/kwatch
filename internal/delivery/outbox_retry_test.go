package delivery

import (
	"context"
	"testing"
	"time"

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
