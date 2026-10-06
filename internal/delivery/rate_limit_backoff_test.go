package delivery

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// waitAfterFailure records one failure and returns the resulting wait.
func waitAfterFailure(
	m *Manager, entry providerEntry, clk *fakeClock, err error,
) time.Duration {
	m.recordProviderFailure(&entry, err)
	return m.pacer.blockedUntil(entry.provider.Name()).Sub(clk.Now())
}

// A 429 that names no wait backs off exponentially, with jitter, up to
// the cap, instead of retrying every five seconds forever.
func TestRateLimitWithoutRetryAfterBacksOffExponentially(t *testing.T) {
	clk := newFakeClock()
	provider := newMessageProvider(nil)
	manager := fakeClockManager(t, clk, provider, nil)
	entry := manager.generation.entries["slack"]
	limited := &ratelimit.Error{Provider: "slack", StatusCode: 429}

	base := initialProviderBackoff
	for i := 0; i < 4; i++ {
		got := waitAfterFailure(manager, entry, clk, limited)
		want := base << i
		assert.GreaterOrEqual(t, got, want*8/10, "failure %d", i+1)
		assert.LessOrEqual(t, got, want*12/10, "failure %d", i+1)
	}
	for i := 0; i < 20; i++ {
		got := waitAfterFailure(manager, entry, clk, limited)
		require.LessOrEqual(t, got, maxProviderBackoff)
	}

	manager.recordProviderSuccess(&entry)
	clk.Advance(2 * maxProviderBackoff)
	got := waitAfterFailure(manager, entry, clk, limited)
	assert.Less(t, got, 2*initialProviderBackoff, "success resets")
}

// A wait the provider names is used as it is, however often it repeats.
func TestRateLimitWithRetryAfterIsNotChanged(t *testing.T) {
	clk := newFakeClock()
	manager := fakeClockManager(t, clk, newMessageProvider(nil), nil)
	entry := manager.generation.entries["slack"]
	named := &ratelimit.Error{StatusCode: 429, RetryAfter: 7 * time.Second}

	for i := 0; i < 3; i++ {
		assert.Equal(t, 7*time.Second,
			waitAfterFailure(manager, entry, clk, named))
	}
}

// A 503 with Retry-After is waited out for as long as it asks.
func TestServiceUnavailableRetryAfterIsHonoured(t *testing.T) {
	clk := newFakeClock()
	manager := fakeClockManager(t, clk, newMessageProvider(nil), nil)
	entry := manager.generation.entries["slack"]
	busy := &transport.RetryAfterError{
		Err: errors.New("503"), RetryAfter: 45 * time.Second,
	}

	assert.Equal(t, 45*time.Second,
		waitAfterFailure(manager, entry, clk, busy))
}
