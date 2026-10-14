package delivery

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/safego"
)

// panickingOutbox panics on its first panics writes, then works.
type panickingOutbox struct {
	*memOutbox
	panics atomic.Int32
}

func (p *panickingOutbox) WriteOutbox(
	puts []OutboxRecord, removes []string,
) error {
	if p.panics.Add(-1) >= 0 {
		panic("store blew up")
	}
	return p.memOutbox.WriteOutbox(puts, removes)
}

func noBackoff(t *testing.T) {
	old := safego.Wait
	safego.Wait = func(context.Context, time.Duration) bool { return true }
	t.Cleanup(func() { safego.Wait = old })
}

func startPanicking(
	t *testing.T, panics int32,
) (*outbox, *panickingOutbox, chan bool) {
	noBackoff(t)
	store := &panickingOutbox{memOutbox: newMemOutbox()}
	store.panics.Store(panics)
	box := newOutbox(store, time.Now)
	sick := make(chan bool, 8)
	box.sick = func(v bool) { sick <- v }
	box.add(deliverJob{kind: jobMessage, msg: "m"}, "slack")
	box.start(context.Background())
	t.Cleanup(func() { require.NoError(t, box.close(5*time.Second)) })
	return box, store, sick
}

func waitFor(t *testing.T, ch <-chan bool) bool {
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("no health report")
		return false
	}
}

func TestOutboxWriterRestartsAfterPanicWithoutLosingBatch(t *testing.T) {
	_, store, _ := startPanicking(t, 1)

	select {
	case <-store.writes:
	case <-time.After(5 * time.Second):
		t.Fatal("the restarted writer never wrote the batch")
	}

	require.Equal(t, 1, store.len(), "the batch survived the panic")
}

func TestOutboxWriterReportsSickThenRecovers(t *testing.T) {
	box, store, sick := startPanicking(t, safego.SickAfter)

	require.True(t, waitFor(t, sick), "repeated panics degrade health")
	require.False(t, waitFor(t, sick), "the next good write recovers it")
	require.Equal(t, 1, store.len())

	box.add(deliverJob{kind: jobMessage, msg: "later"}, "slack")
	<-store.writes
	<-store.writes
	require.Equal(t, 2, store.len(), "the writer keeps working")
}
