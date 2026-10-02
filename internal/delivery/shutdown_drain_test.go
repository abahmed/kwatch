package delivery

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
)

// startedManager runs one provider built by build and returns the manager
// with its lifecycle cancel function.
func startedManager(
	t *testing.T, build func(name string) Provider,
) (*Manager, context.CancelFunc) {
	t.Helper()
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{"slack": {}},
	})
	manager := NewManagerWithDependencies(
		Dependencies{Clock: clock.RealClock{}})
	require.NoError(t, manager.InitRuntime(runtime, func(
		name string, _ map[string]interface{}, _ transport.ProviderContext,
	) Provider {
		return build(name)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, manager.Start(ctx))
	return manager, cancel
}

// stopWorkers ends the lifecycle context and waits for the workers to
// return, as the application does before calling Stop.
func stopWorkers(manager *Manager, cancel context.CancelFunc) {
	cancel()
	manager.mu.Lock()
	done := manager.workers.done
	manager.mu.Unlock()
	<-done
}

func TestManagerStopDeliversJobsQueuedAfterCancellation(t *testing.T) {
	provider := &recordingProvider{messages: make(chan string, 4)}
	manager, cancel := startedManager(t, func(name string) Provider {
		provider.name = name
		return provider
	})
	stopWorkers(manager, cancel)
	manager.Notify("first")
	manager.Notify("second")
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()

	require.NoError(t, manager.Stop(context.Background()))

	require.Equal(t, "first", <-provider.messages)
	require.Equal(t, "second", <-provider.messages)
	require.Equal(t, before,
		metrics.DefaultRegistry().DeliveryDeadLetters.Load())
	select {
	case <-manager.Done():
	default:
		t.Fatal("manager did not report completion after draining")
	}
}

func TestManagerStopDeadLettersJobsItCannotSend(t *testing.T) {
	provider := &blockingProvider{started: make(chan struct{})}
	manager, cancel := startedManager(t, func(name string) Provider {
		provider.name = name
		return provider
	})
	stopWorkers(manager, cancel)
	for _, message := range []string{"a", "b", "c"} {
		manager.Notify(message)
	}
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()
	stopCtx, stopCancel := context.WithCancel(context.Background())
	go func() {
		<-provider.started
		stopCancel()
	}()

	err := manager.Stop(stopCtx)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, before+3,
		metrics.DefaultRegistry().DeliveryDeadLetters.Load())
}

func TestManagerStopWithExpiredContextDeadLettersQueue(t *testing.T) {
	provider := &recordingProvider{messages: make(chan string, 4)}
	manager, cancel := startedManager(t, func(name string) Provider {
		provider.name = name
		return provider
	})
	stopWorkers(manager, cancel)
	manager.Notify("late")
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()
	stopCtx, stopCancel := context.WithCancel(context.Background())
	stopCancel()

	require.Error(t, manager.Stop(stopCtx))

	require.Len(t, provider.messages, 0)
	require.Equal(t, before+1,
		metrics.DefaultRegistry().DeliveryDeadLetters.Load())
}

func TestManagerStopDrainsThroughLiveWorkers(t *testing.T) {
	provider := &recordingProvider{messages: make(chan string, 4)}
	manager, cancel := startedManager(t, func(name string) Provider {
		provider.name = name
		return provider
	})
	defer cancel()
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()
	manager.Notify("live")

	require.NoError(t, manager.Stop(context.Background()))

	require.Equal(t, "live", <-provider.messages)
	require.Equal(t, before,
		metrics.DefaultRegistry().DeliveryDeadLetters.Load())
}

func TestDeadLetterReasonIsBounded(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"shutdown", errDeliveryShutdown, "delivery_shutdown"},
		{"wrapped shutdown",
			fmt.Errorf("stop: %w", errDeliveryShutdown), "delivery_shutdown"},
		{"saturated queue",
			errors.New("delivery queue saturated"), "queue_saturated"},
		{"provider failure", errors.New("HTTP 500"), "delivery_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, deadLetterReason(tc.err))
		})
	}
}
