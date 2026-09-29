package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func runtimeWith(cfg *config.Config) config.RuntimeConfig {
	return config.CompileRuntimeConfig(cfg)
}

func TestMaintenanceAnnotationsEmptyWhenDisabled(t *testing.T) {
	got := maintenanceAnnotations(config.MaintenanceConfig{
		Annotation: "hold", UntilAnnotation: "until",
	})

	require.Equal(t, kube.MaintenanceAnnotations{}, got)
}

func TestMaintenanceAnnotationsCarryConfiguredNames(t *testing.T) {
	got := maintenanceAnnotations(config.MaintenanceConfig{
		Enabled: true, Annotation: "hold", UntilAnnotation: "until",
	})

	require.Equal(t, "hold", got.On)
	require.Equal(t, "until", got.Until)
}

func TestNewActiveProberDisabledBuildsNothing(t *testing.T) {
	deps := &serverDeps{runtime: runtimeWith(&config.Config{})}

	prober, ok := newActiveProber(deps, nil, nil)

	require.False(t, ok)
	require.Nil(t, prober)
}

func TestNewActiveProberEnabledBuildsProber(t *testing.T) {
	deps := &serverDeps{
		runtime: runtimeWith(&config.Config{
			ActiveProbeMonitor: config.ActiveProbeMonitor{
				Enabled:           true,
				IntervalSeconds:   5,
				TimeoutSeconds:    1,
				FailureThreshold:  2,
				HTTP:              []config.HTTPProbeTarget{{Name: "api"}},
				TCP:               []config.TCPProbeTarget{{Name: "db"}},
				DNS:               []config.DNSProbeTarget{{Name: "dns"}},
				ExcludeNamespaces: []string{"locked"},
			},
		}),
		clients: client.ClientSet{Clock: clock.RealClock{}},
	}

	prober, ok := newActiveProber(deps,
		knowledge.NewModel(knowledge.Options{}), nil)

	require.True(t, ok)
	require.NotNil(t, prober)
}

func TestPruneModelReturnsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})

	go func() {
		pruneModel(ctx, knowledge.NewModel(knowledge.Options{}), time.Now)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pruneModel ignored cancellation")
	}
}

func TestCoreClockDelegatesToApplicationClock(t *testing.T) {
	now := time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC)
	c := coreClock{clock.Func(func() time.Time { return now })}

	require.True(t, c.Now().Equal(now))
	select {
	case <-c.After(time.Millisecond):
	case <-time.After(5 * time.Second):
		t.Fatal("After never fired")
	}
}

func TestNewDetectorRegistryUsesSyncPredicate(t *testing.T) {
	registry := newDetectorRegistry(func(knowledge.Kind) bool {
		return true
	})
	model := knowledge.NewModel(knowledge.Options{})

	registry.Evaluate(model, time.Now(),
		knowledge.NewEntityID(kube.KindNode, "", "n1"))

	require.NotNil(t, registry)
	require.NotNil(t, newReasoner())
}
