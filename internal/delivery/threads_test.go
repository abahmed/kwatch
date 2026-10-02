package delivery

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// threadedProvider keeps thread ids like a chat provider with threads.
type threadedProvider struct {
	fakeProvider
	mu      sync.Mutex
	threads map[string]string
}

func (p *threadedProvider) SnapshotThreads() map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]string, len(p.threads))
	for key, id := range p.threads {
		out[key] = id
	}
	return out
}

func (p *threadedProvider) RestoreThreads(saved map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.threads == nil {
		p.threads = make(map[string]string)
	}
	for key, id := range saved {
		p.threads[key] = id
	}
}

func TestReconfigurationCarriesThreadsToNewProviders(t *testing.T) {
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{"slack": {}},
	})
	built := []*threadedProvider{}
	factory := func(
		string, map[string]interface{}, transport.ProviderContext,
	) Provider {
		p := &threadedProvider{}
		built = append(built, p)
		return p
	}
	manager := NewManagerWithDependencies(
		Dependencies{Clock: clock.RealClock{}})
	require.NoError(t, manager.InitRuntime(runtime, factory))
	require.NoError(t, manager.Start(context.Background()))
	t.Cleanup(func() { shutdownManager(manager) })
	built[0].RestoreThreads(map[string]string{"pod/web": "1700.01"})

	require.NoError(t, manager.InitRuntime(runtime, factory))

	require.Len(t, built, 2)
	assert.Equal(t, map[string]string{"pod/web": "1700.01"},
		built[1].SnapshotThreads())
}
