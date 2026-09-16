package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

type recordingProvider struct {
	name     string
	messages chan string
}

func (p *recordingProvider) Name() string { return p.name }

func (p *recordingProvider) SendMessage(
	_ context.Context,
	message string,
) error {
	p.messages <- message
	return nil
}

func (p *recordingProvider) SendEvent(
	_ context.Context,
	_ *event.Event,
) error {
	return nil
}

func TestManagerAddProviderAfterStartUsesStoredQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewManagerWithDependencies(Dependencies{Clock: clock.RealClock{}})
	manager.Start(ctx)

	provider := &recordingProvider{
		name:     "late-provider",
		messages: make(chan string, 1),
	}
	manager.AddProvider(provider)
	manager.Notify("late registration")

	select {
	case message := <-provider.messages:
		require.Equal(t, "late registration", message)
	case <-time.After(time.Second):
		t.Fatal("late provider did not receive a queued message")
	}

	manager.shutdown()
}

func TestManagerReconfigurationRestartsProviderWorkers(t *testing.T) {
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{
			"slack": {},
		},
	})
	created := make([]*recordingProvider, 0, 2)
	factory := func(
		name string,
		_ map[string]interface{},
		_ transport.ProviderContext,
	) Provider {
		provider := &recordingProvider{
			name: name, messages: make(chan string, 1),
		}
		created = append(created, provider)
		return provider
	}
	manager := NewManagerWithDependencies(Dependencies{Clock: clock.RealClock{}})
	manager.InitRuntime(runtime, factory)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)

	manager.InitRuntime(runtime, factory)
	manager.Notify("after reconfiguration")
	if len(created) != 2 {
		t.Fatalf("factory created %d providers, want 2", len(created))
	}
	select {
	case message := <-created[1].messages:
		require.Equal(t, "after reconfiguration", message)
	case <-time.After(time.Second):
		t.Fatal("reconfigured provider did not receive a message")
	}
	manager.shutdown()
}

func TestManagerReconfigurationDoesNotUseOldGeneration(t *testing.T) {
	runtime := config.RuntimeConfigFor(&config.Config{
		Alert: map[string]map[string]interface{}{
			"slack": {},
		},
	})
	created := make([]*recordingProvider, 0, 2)
	factory := func(
		name string,
		_ map[string]interface{},
		_ transport.ProviderContext,
	) Provider {
		provider := &recordingProvider{
			name: name, messages: make(chan string, 1),
		}
		created = append(created, provider)
		return provider
	}
	manager := NewManagerWithDependencies(Dependencies{Clock: clock.RealClock{}})
	manager.InitRuntime(runtime, factory)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	manager.InitRuntime(runtime, factory)
	manager.Notify("new generation")

	select {
	case message := <-created[0].messages:
		t.Fatalf("old provider received %q", message)
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case message := <-created[1].messages:
		require.Equal(t, "new generation", message)
	case <-time.After(time.Second):
		t.Fatal("new provider did not receive a message")
	}
	manager.shutdown()
}

func TestManagerStopBeforeStartIsSafe(t *testing.T) {
	manager := NewManagerWithDependencies(Dependencies{Clock: clock.RealClock{}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("Stop before Start returned error: %v", err)
	}
}

func TestManagerStopIsExplicitAfterContextCancellation(t *testing.T) {
	manager := NewManagerWithDependencies(Dependencies{Clock: clock.RealClock{}})
	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	cancel()

	stopCtx, stopCancel := context.WithTimeout(
		context.Background(), time.Second,
	)
	defer stopCancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}
