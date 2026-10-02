package delivery

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// fakeClock is a settable clock shared by the manager and its waits.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// advancingSleep waits by moving the fake clock, so hours of backoff
// pass at once.
func advancingSleep(c *fakeClock) func(context.Context, time.Duration) bool {
	return func(ctx context.Context, d time.Duration) bool {
		if ctx.Err() != nil {
			return false
		}
		c.Advance(d)
		return true
	}
}

// messageProvider records every incident message it accepts. fail, when
// set, decides each call's error.
type messageProvider struct {
	name  string
	fail  func(call int, m notification.Message) error
	mu    sync.Mutex
	calls int
	sent  chan notification.Message
}

func newMessageProvider(
	fail func(call int, m notification.Message) error,
) *messageProvider {
	return &messageProvider{fail: fail, sent: make(chan notification.Message,
		4096)}
}

func (p *messageProvider) Name() string { return p.name }

func (p *messageProvider) SendMessage(_ context.Context, msg string) error {
	return p.SendIncident(context.Background(),
		notification.Message{Key: "message", Note: msg})
}

func (p *messageProvider) SendIncident(
	_ context.Context, m notification.Message,
) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if p.fail != nil {
		if err := p.fail(call, m); err != nil {
			return err
		}
	}
	p.sent <- m
	return nil
}

func (p *messageProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// receive reads the next accepted message, bounded.
func (p *messageProvider) receive(t *testing.T) notification.Message {
	t.Helper()
	select {
	case m := <-p.sent:
		return m
	case <-time.After(10 * time.Second):
		t.Fatal("provider received nothing")
		return notification.Message{}
	}
}

// fakeClockManager builds a manager with one "slack" provider on the
// fake clock, one attempt per round and waits that move the clock.
func fakeClockManager(
	t *testing.T, clk *fakeClock, provider Provider, store OutboxStore,
) *Manager {
	t.Helper()
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{"slack": {}},
	})
	manager := NewManagerWithDependencies(Dependencies{Clock: clk})
	require.NoError(t, manager.InitRuntime(runtime, func(
		name string, _ map[string]interface{}, _ transport.ProviderContext,
	) Provider {
		if named, ok := provider.(*messageProvider); ok {
			named.name = name
		}
		return provider
	}))
	for name, entry := range manager.generation.entries {
		entry.retry = retryConfig{maxAttempts: 1, delay: time.Millisecond}
		manager.generation.entries[name] = entry
	}
	manager.sleepFn = advancingSleep(clk)
	if store != nil {
		require.NoError(t, manager.AttachOutbox(store))
	}
	return manager
}

// startManager starts delivery and stops it when the test ends.
func startManager(t *testing.T, manager *Manager) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, manager.Start(ctx))
	t.Cleanup(func() {
		stopCtx, stop := context.WithTimeout(context.Background(),
			5*time.Second)
		defer stop()
		_ = manager.Stop(stopCtx)
		cancel()
	})
}

// revision is one message of conversation key.
func revision(key string, rev int, note string) notification.Message {
	return notification.Message{
		Key: key, Revision: rev, Status: notification.StatusCritical,
		Title: key + " failed", Note: note,
	}
}

func resolvedRevision(
	key string, rev int, note string, opening *notification.Message,
) notification.Message {
	m := revision(key, rev, note)
	m.Status = notification.StatusResolved
	m.Opening = opening
	return m
}

// newManagerFor builds a manager whose one provider is configured under
// providerName, with fast pacing.
func newManagerFor(
	t *testing.T, providerName string, provider *scriptedProvider,
) *Manager {
	t.Helper()
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{providerName: {}},
	})
	manager := NewManagerWithDependencies(
		Dependencies{Clock: clock.RealClock{}})
	require.NoError(t, manager.InitRuntime(runtime, func(
		name string, _ map[string]interface{}, _ transport.ProviderContext,
	) Provider {
		provider.name = name
		return provider
	}))
	manager.pacer.interval = time.Millisecond
	return manager
}
