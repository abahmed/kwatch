package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// stallWindow is longer than the application's 40s stall deadline.
const stallWindow = 2 * time.Minute

func TestManagerReportsProgressWhileWaitingOutRetryAfter(t *testing.T) {
	clk := newFakeClock()
	provider := newMessageProvider(func(call int, _ notification.Message) error {
		if call == 1 {
			return &transport.RetryAfterError{
				Err: errors.New("429"), RetryAfter: time.Minute,
			}
		}
		return nil
	})
	manager := fakeClockManager(t, clk, provider, nil)
	waiting, wake := make(chan time.Duration, 1), make(chan struct{})
	manager.sleepFn = func(ctx context.Context, d time.Duration) bool {
		waiting <- d
		<-wake
		return advancingSleep(clk)(ctx, d)
	}
	startManager(t, manager)
	manager.NotifyIncident(revision("a", 1, "🔴 a is down"))

	require.Equal(t, time.Minute, <-waiting, "the Retry-After is honoured")
	clk.Advance(stallWindow)
	assert.False(t, manager.LastProgress().Before(clk.Now()),
		"a worker waiting out a provider is not stalled")
	close(wake)
	assert.Equal(t, "a", provider.receive(t).Key)
}

func TestManagerReportsProgressWhileRequestHangs(t *testing.T) {
	clk := newFakeClock()
	inFlight, release := make(chan struct{}), make(chan struct{})
	provider := newMessageProvider(func(call int, _ notification.Message) error {
		if call == 1 {
			close(inFlight)
			<-release
		}
		return nil
	})
	manager := fakeClockManager(t, clk, provider, nil)
	startManager(t, manager)
	manager.NotifyIncident(revision("a", 1, "🔴 a is down"))

	<-inFlight
	clk.Advance(stallWindow)
	assert.False(t, manager.LastProgress().Before(clk.Now()),
		"a request in flight is progress")
	close(release)
	assert.Equal(t, "a", provider.receive(t).Key)
}

func TestRateLimitBlocksTheProviderForTheNextJob(t *testing.T) {
	clk := newFakeClock()
	start := clk.Now()
	var callTimes []time.Time
	provider := newMessageProvider(func(call int, _ notification.Message) error {
		callTimes = append(callTimes, clk.Now())
		if call == 1 {
			return &ratelimit.Error{StatusCode: 429, RetryAfter: 30 * time.Second}
		}
		return nil
	})
	manager := fakeClockManager(t, clk, provider, nil)
	manager.NotifyIncident(revision("a", 1, "🔴 a is down"))
	manager.NotifyIncident(revision("b", 1, "🔴 b is down"))
	startManager(t, manager)

	assert.Equal(t, "a", provider.receive(t).Key,
		"one attempt per round, and the wait did not spend it")
	assert.Equal(t, "b", provider.receive(t).Key)
	require.Len(t, callTimes, 3)
	blocked := start.Add(30 * time.Second)
	assert.Equal(t, blocked, manager.pacer.blockedUntil("slack"))
	assert.False(t, callTimes[1].Before(blocked), "a waited out the 429")
	assert.False(t, callTimes[2].Before(blocked), "b waited as well")
}
