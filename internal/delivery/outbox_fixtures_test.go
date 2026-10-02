package delivery

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// memOutbox is an in-memory OutboxStore. writes receives one signal per
// successful WriteOutbox call.
type memOutbox struct {
	mu      sync.Mutex
	records map[string]OutboxRecord
	writes  chan struct{}
}

func newMemOutbox(records ...OutboxRecord) *memOutbox {
	store := &memOutbox{
		records: make(map[string]OutboxRecord),
		writes:  make(chan struct{}, 64),
	}
	for _, record := range records {
		store.records[record.ID] = record
	}
	return store
}

func (m *memOutbox) LoadOutbox() ([]OutboxRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]OutboxRecord, 0, len(m.records))
	for _, record := range m.records {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memOutbox) WriteOutbox(puts []OutboxRecord, removes []string) error {
	m.mu.Lock()
	for _, record := range puts {
		m.records[record.ID] = record
	}
	for _, id := range removes {
		delete(m.records, id)
	}
	m.mu.Unlock()
	select {
	case m.writes <- struct{}{}:
	default:
	}
	return nil
}

func (m *memOutbox) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.records)
}

// copy is the store as a crash leaves it: what was written survives.
func (m *memOutbox) copy() *memOutbox {
	records, _ := m.LoadOutbox()
	return newMemOutbox(records...)
}

// waitForRecords waits, bounded, until the store holds n records.
func (m *memOutbox) waitForRecords(t *testing.T, n int) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for m.len() != n {
		select {
		case <-m.writes:
		case <-deadline.C:
			t.Fatalf("outbox has %d records, want %d", m.len(), n)
		}
	}
}

// scriptedProvider answers each send with send(call index, text).
type scriptedProvider struct {
	name  string
	mu    sync.Mutex
	calls int
	send  func(ctx context.Context, call int, text string) error
	sent  chan string
}

func newScriptedProvider(
	send func(ctx context.Context, call int, text string) error,
) *scriptedProvider {
	return &scriptedProvider{send: send, sent: make(chan string, 64)}
}

func (p *scriptedProvider) Name() string { return p.name }

func (p *scriptedProvider) SendMessage(ctx context.Context, msg string) error {
	return p.record(ctx, msg)
}

func (p *scriptedProvider) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	return p.record(ctx, m.Key)
}

func (p *scriptedProvider) record(ctx context.Context, text string) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if p.send != nil {
		if err := p.send(ctx, call, text); err != nil {
			return err
		}
	}
	p.sent <- text
	return nil
}

// receive reads the next accepted send, bounded.
func (p *scriptedProvider) receive(t *testing.T) string {
	t.Helper()
	select {
	case text := <-p.sent:
		return text
	case <-time.After(5 * time.Second):
		t.Fatal("provider received nothing")
		return ""
	}
}

// outboxManager builds a manager with one "slack" provider, fast pacing
// and store attached, without starting it.
func outboxManager(
	t *testing.T, provider *scriptedProvider, store OutboxStore,
) *Manager {
	t.Helper()
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{"slack": {}},
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
	if store != nil {
		require.NoError(t, manager.AttachOutbox(store))
	}
	return manager
}

func messageRecord(id, text string, queued time.Time) OutboxRecord {
	return OutboxRecord{
		Version: outboxRecordVersion, ID: id, Provider: "slack",
		Message: text, Queued: queued,
	}
}
