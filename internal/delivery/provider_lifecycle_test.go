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

type blockingProvider struct {
	name    string
	started chan struct{}
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

func (p *blockingProvider) Name() string { return p.name }

func (p *blockingProvider) SendMessage(ctx context.Context, _ string) error {
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (p *blockingProvider) SendEvent(
	ctx context.Context,
	_ *event.Event,
) error {
	return p.SendMessage(ctx, "event")
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

func TestManagerQueuesMessagesUntilWorkersStart(t *testing.T) {
	provider := &recordingProvider{
		name:     "slack",
		messages: make(chan string, 1),
	}
	manager := managerWithEntries([]providerEntry{{
		provider: provider,
	}})

	manager.Notify("queued before start")
	select {
	case <-provider.messages:
		t.Fatal("message was delivered before workers started")
	default:
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, manager.Start(ctx))
	select {
	case message := <-provider.messages:
		require.Equal(t, "queued before start", message)
	case <-time.After(time.Second):
		t.Fatal("pending message was not delivered after Start")
	}
	require.NoError(t, manager.Stop(context.Background()))
}

func TestManagerRecordsJobsRemainingAfterShutdownTimeout(t *testing.T) {
	provider := &blockingProvider{
		name:    "slack",
		started: make(chan struct{}),
	}
	manager := managerWithEntries([]providerEntry{{provider: provider}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, manager.Start(ctx))
	manager.Notify("first")
	manager.Notify("second")
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start sending")
	}

	stopCtx, stopCancel := context.WithTimeout(
		context.Background(), 10*time.Millisecond,
	)
	defer stopCancel()
	require.Error(t, manager.Stop(stopCtx))
	require.NotEmpty(t, manager.DeadLetters())
}

func TestManagerRetainsNotificationsDuringReconfiguration(t *testing.T) {
	first := &recordingProvider{
		name: "first", messages: make(chan string, 1),
	}
	second := &recordingProvider{
		name: "second", messages: make(chan string, 1),
	}
	manager := managerWithEntries([]providerEntry{{provider: first}})

	manager.mu.Lock()
	manager.started = true
	manager.reconfiguring = true
	manager.stopped = true
	manager.mu.Unlock()
	manager.Notify("during reconfiguration")
	manager.mu.Lock()
	manager.reconfiguring = false
	manager.stopped = false
	manager.generation = newProviderGeneration([]providerEntry{{
		provider: second,
		ch:       make(chan deliverJob, channelCap),
	}})
	manager.started = false
	manager.mu.Unlock()
	require.NoError(t, manager.Start(context.Background()))
	require.Eventually(t, func() bool {
		select {
		case message := <-second.messages:
			return message == "during reconfiguration"
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	_ = manager.Stop(context.Background())
}

func TestManagerReportsReconfigurationResultOnce(t *testing.T) {
	manager := NewManagerWithDependencies(Dependencies{
		Clock: clock.RealClock{},
	})
	result := context.Canceled
	done := make(chan struct{})
	close(done)
	manager.mu.Lock()
	manager.reconfigureDone = done
	manager.reconfigureErr = result
	manager.reconfigureWait = true
	manager.mu.Unlock()

	require.ErrorIs(t,
		manager.WaitForReconfiguration(context.Background()), result)
	require.ErrorIs(t,
		manager.WaitForReconfiguration(context.Background()),
		ErrNoReconfiguration,
	)
}

func TestManagerReportsSuccessfulReconfiguration(t *testing.T) {
	manager := NewManagerWithDependencies(Dependencies{
		Clock: clock.RealClock{},
	})
	done := make(chan struct{})
	close(done)
	manager.mu.Lock()
	manager.reconfigureDone = done
	manager.reconfigureWait = true
	manager.mu.Unlock()

	require.NoError(t,
		manager.WaitForReconfiguration(context.Background()))
}
