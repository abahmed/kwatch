package delivery

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// stoppableProvider counts the Stop calls delivery makes.
type stoppableProvider struct {
	recordingProvider
	stops atomic.Int32
}

func (p *stoppableProvider) Stop() { p.stops.Add(1) }

// A provider that keeps work running after a send (a close retried in
// the background) is stopped once, when delivery shuts down.
func TestManagerStopStopsProvidersWithBackgroundWork(t *testing.T) {
	provider := &stoppableProvider{}
	provider.messages = make(chan string, 4)
	manager, cancel := startedManager(t, func(name string) Provider {
		provider.name = name
		return provider
	})
	stopWorkers(manager, cancel)
	require.Zero(t, provider.stops.Load())

	require.NoError(t, manager.Stop(context.Background()))

	require.EqualValues(t, 1, provider.stops.Load())
}

// A reconfiguration replaces the providers but does not stop the old
// ones: what they still retry is bounded and must not be given up.
func TestReconfigurationDoesNotStopTheOldProviders(t *testing.T) {
	first := &stoppableProvider{}
	first.messages = make(chan string, 4)
	manager, cancel := startedManager(t, func(name string) Provider {
		first.name = name
		return first
	})
	defer cancel()
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{"slack": {}},
	})
	second := &stoppableProvider{}
	second.messages = make(chan string, 4)
	require.NoError(t, manager.InitRuntime(runtime, func(
		name string, _ map[string]interface{}, _ transport.ProviderContext,
	) Provider {
		second.name = name
		return second
	}))

	require.Zero(t, first.stops.Load())
	require.NoError(t, manager.Stop(context.Background()))
	require.EqualValues(t, 1, second.stops.Load())
}
