package delivery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// clockProvider records the fake time of each request. It fails with
// err: every request, or only the first failFirst when that is set.
type clockProvider struct {
	name      string
	clk       *fakeClock
	err       error
	failFirst int
	mu        sync.Mutex
	at        []time.Time
}

func (p *clockProvider) Name() string { return p.name }

func (p *clockProvider) SendMessage(context.Context, string) error {
	return p.SendIncident(context.Background(), notification.Message{})
}

func (p *clockProvider) SendIncident(
	context.Context, notification.Message,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.at = append(p.at, p.clk.Now())
	if p.failFirst > 0 && len(p.at) > p.failFirst {
		return nil
	}
	return p.err
}

func (p *clockProvider) calls() []time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time(nil), p.at...)
}

// clockPager is a clockProvider that pages: it skips plain messages.
type clockPager struct{ *clockProvider }

func (clockPager) SkipsPlainMessages() bool { return true }

func clockedFallbackPair(
	clk *fakeClock, fb *clockProvider,
) (*Manager, *providerEntry) {
	primary := &errorRecorderProvider{name: "Primary",
		err: errors.New("timeout")}
	m, entry := fallbackPair(primary, fb)
	m.now = clk.Now
	m.sleepFn = advancingSleep(clk)
	return m, entry
}

// A fallback that asked to be left alone is not sent to, and not waited
// out either: the job is retried later, as when there is no fallback.
func TestBlockedFallbackIsNotSentToBeforeItsBlockEnds(t *testing.T) {
	clk := newFakeClock()
	fb := &clockProvider{name: "Fallback", clk: clk}
	m, entry := clockedFallbackPair(clk, fb)
	start := clk.Now()
	m.pacer.block("Fallback", start.Add(time.Minute))

	got := m.deliver(context.Background(), entry, incidentJob("k", "ns"))

	assert.Equal(t, outcomeRetryLater, got)
	assert.Empty(t, fb.calls(), "the blocked fallback was sent to")
	assert.Equal(t, start, clk.Now(), "the fallback's block was waited out")
	assert.Equal(t, openFate(0), m.openFateOf(entry.lookupName(), "k"),
		"the primary is still owed the opening")
}

// A primary that rejected the payload for good is not retried, so its
// fallback's block is still waited out, never sent through.
func TestRejectedJobWaitsOutFallbackBlock(t *testing.T) {
	clk := newFakeClock()
	fb := &clockProvider{name: "Fallback", clk: clk}
	primary := &errorRecorderProvider{name: "Primary",
		err: transport.Permanent(errors.New("bad payload"))}
	m, entry := fallbackPair(primary, fb)
	m.now, m.sleepFn = clk.Now, advancingSleep(clk)
	end := clk.Now().Add(time.Minute)
	m.pacer.block("Fallback", end)

	got := m.deliver(context.Background(), entry, incidentJob("k", "ns"))

	assert.Equal(t, outcomeDelivered, got)
	require.Len(t, fb.calls(), 1)
	assert.False(t, fb.calls()[0].Before(end))
}

// A blocked fallback does not hold the primary's worker: the job is
// retried on the primary, which recovers, and the next job queued for
// the primary goes out long before the fallback's block ends.
func TestBlockedFallbackDoesNotHoldPrimaryWorker(t *testing.T) {
	clk := newFakeClock()
	fb := &clockProvider{name: "Fallback", clk: clk}
	primary := &clockProvider{name: "Primary", clk: clk,
		err: errors.New("timeout"), failFirst: 1}
	m, entry := fallbackPair(primary, fb)
	m.now, m.sleepFn = clk.Now, advancingSleep(clk)
	end := clk.Now().Add(time.Minute)
	m.pacer.block("Fallback", end)
	ctx := context.Background()

	require.True(t, m.workOne(ctx, entry, incidentJob("a", "ns")))
	require.True(t, m.workOne(ctx, entry, incidentJob("b", "ns")))

	assert.Empty(t, fb.calls(), "the fallback was sent to while blocked")
	sent := primary.calls()
	require.Len(t, sent, 3, "a failed once, then a and b were delivered")
	assert.True(t, sent[2].Before(end),
		"the primary's next job waited for the fallback's block")
}

// While the primary stays down the deferred job goes to the fallback once
// its block ends, never before, and the primary is retried meanwhile.
func TestDeferredJobReachesFallbackAfterItsBlock(t *testing.T) {
	clk := newFakeClock()
	fb := &clockProvider{name: "Fallback", clk: clk}
	primary := &clockProvider{name: "Primary", clk: clk,
		err: errors.New("timeout")}
	m, entry := fallbackPair(primary, fb)
	m.now, m.sleepFn = clk.Now, advancingSleep(clk)
	end := clk.Now().Add(time.Minute)
	m.pacer.block("Fallback", end)

	require.True(t, m.workOne(context.Background(), entry,
		incidentJob("k", "ns")))

	require.Len(t, fb.calls(), 1)
	assert.False(t, fb.calls()[0].Before(end),
		"the fallback was sent to before its block ended")
	assert.Greater(t, len(primary.calls()), 1,
		"the primary was not retried while the fallback was blocked")
	assert.Equal(t, openLost, m.openFateOf(entry.lookupName(), "k"),
		"the primary never heard the opening the fallback took")
}

// A page whose paging fallback is blocked is kept and retried, then
// paged through the fallback once its block ends; it is never lost.
func TestPageIsNotLostWhileFallbackIsBlocked(t *testing.T) {
	clk := newFakeClock()
	pager := clockPager{&clockProvider{name: "Fallback", clk: clk}}
	chat := &clockProvider{name: "Primary", clk: clk,
		err: errors.New("timeout")}
	m, entry := fallbackPair(chat, pager)
	m.now, m.sleepFn = clk.Now, advancingSleep(clk)
	var lost []string
	m.AttachPageObserver(func(key string) { lost = append(lost, key) })
	end := clk.Now().Add(time.Minute)
	m.pacer.block("Fallback", end)

	require.True(t, m.workOne(context.Background(), entry,
		pageAnnounce("p")))

	require.Len(t, pager.calls(), 1, "the page never reached the pager")
	assert.False(t, pager.calls()[0].Before(end))
	assert.Greater(t, len(chat.calls()), 1,
		"the primary was not retried while the pager was blocked")
	assert.Empty(t, lost, "the page was reported lost")
}

// A failing fallback backs off and is reported like any provider.
func TestFallbackFailureBlocksAndMarksFallback(t *testing.T) {
	clk := newFakeClock()
	fb := &clockProvider{name: "Fallback", clk: clk,
		err: errors.New("timeout")}
	m, entry := clockedFallbackPair(clk, fb)

	m.deliver(context.Background(), entry, incidentJob("k", "ns"))

	assert.True(t, m.pacer.blockedUntil("Fallback").After(clk.Now()))
	fbEntry := managerEntries(m)[1]
	assert.Equal(t, reasonUnavailable, healthReason(m, &fbEntry))
}
