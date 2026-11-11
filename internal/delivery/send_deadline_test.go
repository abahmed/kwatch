package delivery

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

// ctxProvider reports the context of each request it gets and, when hold
// is set, ignores that context until released (a hung provider call).
type ctxProvider struct {
	name     string
	hold     chan struct{}
	entered  chan struct{}
	mu       sync.Mutex
	deadline time.Time
	hasLimit bool
}

func (p *ctxProvider) Name() string { return p.name }

func (p *ctxProvider) SendMessage(ctx context.Context, _ string) error {
	return p.SendIncident(ctx, notification.Message{})
}

func (p *ctxProvider) SendIncident(
	ctx context.Context, _ notification.Message,
) error {
	p.mu.Lock()
	p.deadline, p.hasLimit = ctx.Deadline()
	p.mu.Unlock()
	if p.hold != nil {
		p.entered <- struct{}{}
		<-p.hold
	}
	return nil
}

func ctxProviderEntry(p *ctxProvider) (*Manager, *providerEntry) {
	m := managerWithEntries([]providerEntry{
		{provider: p, retry: fastRetry},
	})
	return m, &managerEntries(m)[0]
}

// Provider requests carry a deadline; the default is generous and not
// below the transport's 30 second HTTP timeout.
func TestProviderRequestHasSendDeadline(t *testing.T) {
	p := &ctxProvider{name: "P"}
	m, entry := ctxProviderEntry(p)

	m.deliver(t.Context(), entry, incidentJob("k", "ns"))

	require.True(t, p.hasLimit, "the request context has a deadline")
	left := time.Until(p.deadline)
	assert.Greater(t, left, 30*time.Second)
	assert.LessOrEqual(t, left, sendTimeout)
	assert.GreaterOrEqual(t, sendTimeout, 30*time.Second)
}

// A request that ignores its context past the send deadline no longer
// counts as progress, though its worker is busy.
func TestHungSendIsNotProgress(t *testing.T) {
	p := &ctxProvider{name: "P", hold: make(chan struct{}),
		entered: make(chan struct{}, 1)}
	m, entry := ctxProviderEntry(p)
	clk := newFakeClock()
	m.now = clk.Now
	go m.deliver(t.Context(), entry, incidentJob("k", "ns"))
	<-p.entered
	defer close(p.hold)
	m.busy.Add(1)
	defer m.busy.Add(-1)

	assert.Equal(t, clk.Now(), m.LastProgress(), "a young send is progress")
	clk.Advance(sendTimeout + sendStallGrace + time.Second)

	assert.True(t, m.LastProgress().Before(clk.Now()),
		"a send past its deadline is not progress")
}
