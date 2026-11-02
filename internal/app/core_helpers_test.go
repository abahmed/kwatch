package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory"
	inventorykube "github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

func runtimeWith(cfg *config.Config) config.RuntimeConfig {
	return config.CompileRuntimeConfig(cfg)
}

func TestMaintenanceAnnotationsEmptyWhenDisabled(t *testing.T) {
	got := maintenanceAnnotations(config.MaintenanceConfig{
		Annotation: "hold", UntilAnnotation: "until",
	})

	require.Equal(t, inventorykube.MaintenanceAnnotations{}, got)
}

func TestMaintenanceAnnotationsCarryConfiguredNames(t *testing.T) {
	got := maintenanceAnnotations(config.MaintenanceConfig{
		Enabled: true, Annotation: "hold", UntilAnnotation: "until",
	})

	require.Equal(t, "hold", got.On)
	require.Equal(t, "until", got.Until)
}

func TestNewActiveProberDisabledBuildsNothing(t *testing.T) {
	deps := &serverDeps{
		entitlements: feature.ProEntitlements(),
		runtime:      runtimeWith(&config.Config{}),
	}

	prober, ok := newActiveProber(deps, nil, nil)

	require.False(t, ok)
	require.Nil(t, prober)
}

func TestNewActiveProberEnabledBuildsProber(t *testing.T) {
	deps := &serverDeps{entitlements: feature.ProEntitlements(),
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
		clients: kubeclient.ClientSet{Clock: clock.RealClock{}},
	}

	prober, ok := newActiveProber(deps,
		inventory.NewModel(inventory.Options{}), nil)

	require.True(t, ok)
	require.NotNil(t, prober)
}

func TestPruneModelReturnsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})

	go func() {
		pruneModel(ctx, inventory.NewModel(inventory.Options{}), time.Now)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pruneModel ignored cancellation")
	}
}

func TestPipelineClockDelegatesToApplicationClock(t *testing.T) {
	now := time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC)
	c := pipelineClock{clock.Func(func() time.Time { return now })}

	require.True(t, c.Now().Equal(now))
	select {
	case <-c.After(time.Millisecond):
	case <-time.After(5 * time.Second):
		t.Fatal("After never fired")
	}
}

func TestNewDetectorRegistryUsesSyncPredicate(t *testing.T) {
	registry := newDetectorRegistry(func(inventory.Kind) bool {
		return true
	}, nil, feature.ProEntitlements(), nil)
	model := inventory.NewModel(inventory.Options{})

	registry.Evaluate(model, time.Now(),
		inventory.CoreID(inventorykube.KindNode, "", "n1"))

	require.NotNil(t, registry)
}

// The registry reads the source's freshness: a CronJob whose watch went
// stale is undecided instead of overdue, and raises once it is fresh.
func TestNewDetectorRegistryHoldsScheduleWhileTheWatchIsStale(
	t *testing.T,
) {
	fresh := false
	registry := newDetectorRegistry(nil, func(inventory.Kind) bool {
		return fresh
	}, feature.ProEntitlements(), nil)
	model := inventory.NewModel(inventory.Options{})
	id := inventory.CoreID(inventorykube.KindCronJob, "default", "nightly")
	next := time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)
	model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test",
		At: next.Add(-time.Hour), Entity: id,
		Attributes: map[string]inventory.Value{
			inventorykube.AttrNextRun: inventory.Time(next),
		},
	})
	late := next.Add(10 * time.Minute)

	require.Empty(t, registry.Evaluate(model, late, id).Findings)
	fresh = true
	require.NotEmpty(t, registry.Evaluate(model, late, id).Findings)
}

func TestPipelineBuildFreshBeforeTheSourceExists(t *testing.T) {
	p := &pipelineBuild{}

	require.True(t, p.fresh(inventorykube.KindCronJob))
}
