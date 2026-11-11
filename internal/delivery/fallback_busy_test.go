package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reserveSlots takes the provider's next n send slots, as its own
// queued sends do.
func reserveSlots(m *Manager, provider string, n int) {
	m.pacer.mu.Lock()
	defer m.pacer.mu.Unlock()
	for i := 0; i < n; i++ {
		m.pacer.reserve(provider, m.nowTime())
	}
}

// A fallback whose next slots are taken by its own sends is healthy,
// only busy. The failed job takes the next free slot and is sent there,
// instead of going back to the failing primary's backoff.
func TestBusyFallbackIsWaitedForNotTakenAsBlocked(t *testing.T) {
	clk := newFakeClock()
	fb := &clockProvider{name: "Fallback", clk: clk}
	m, entry := clockedFallbackPair(clk, fb)
	start := clk.Now()
	// The first slot is free now; the next two are taken.
	reserveSlots(m, "Fallback", 3)
	interval := m.pacer.sendInterval()

	got := m.deliver(context.Background(), entry, incidentJob("k", "ns"))

	assert.Equal(t, outcomeDelivered, got)
	require.Len(t, fb.calls(), 1, "the busy fallback was not used")
	assert.Equal(t, start.Add(3*interval), fb.calls()[0],
		"the job must take the slot behind the fallback's own sends")
}

// The wait behind a busy fallback is bounded: with more than a few
// slots taken the job is retried later and nothing is reserved.
func TestVeryBusyFallbackIsNotWaitedFor(t *testing.T) {
	clk := newFakeClock()
	fb := &clockProvider{name: "Fallback", clk: clk}
	m, entry := clockedFallbackPair(clk, fb)
	start := clk.Now()
	reserveSlots(m, "Fallback", shortSlotIntervals+2)
	m.pacer.mu.Lock()
	reserved := m.pacer.last["Fallback"]
	m.pacer.mu.Unlock()

	got := m.deliver(context.Background(), entry, incidentJob("k", "ns"))

	assert.Equal(t, outcomeRetryLater, got)
	assert.Empty(t, fb.calls())
	assert.Equal(t, start, clk.Now(), "the busy fallback was waited out")
	m.pacer.mu.Lock()
	defer m.pacer.mu.Unlock()
	assert.Equal(t, reserved, m.pacer.last["Fallback"],
		"a slot was reserved for a job that was not sent")
}

// A block of at most one interval is waited out, as before.
func TestShortFallbackBlockIsWaitedOut(t *testing.T) {
	clk := newFakeClock()
	fb := &clockProvider{name: "Fallback", clk: clk}
	m, entry := clockedFallbackPair(clk, fb)
	end := clk.Now().Add(m.pacer.sendInterval() / 2)
	m.pacer.block("Fallback", end)

	got := m.deliver(context.Background(), entry, incidentJob("k", "ns"))

	assert.Equal(t, outcomeDelivered, got)
	require.Len(t, fb.calls(), 1)
	assert.False(t, fb.calls()[0].Before(end))
	assert.True(t, fb.calls()[0].Before(end.Add(time.Second)))
}
