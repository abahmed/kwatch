package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/abahmed/kwatch/internal/config"
)

// watcherAfterBootstrap applies the startup overlay as bootstrap does,
// lets change alter the KwatchConfig, then starts the watcher as the
// leader session does. It reports whether the watcher asked for a
// restart once it was running.
func watcherAfterBootstrap(
	t *testing.T, client dynamic.Interface,
	change func(dynamic.ResourceInterface),
) bool {
	t.Helper()
	t.Setenv("POD_NAMESPACE", "kwatch")
	cfg, outcome, err := applyStartupCRD(context.Background(), crdBase(),
		client, func() (*config.Config, error) { return crdBase(), nil },
		noLastGood, nil)
	require.NoError(t, err)
	require.True(t, outcome.listed)
	change(client.Resource(kwatchConfigResource).Namespace("kwatch"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var restarted atomic.Bool
	deps := componentDeps()
	deps.runtime = config.CompileRuntimeConfig(cfg)
	deps.clients.Dynamic = client
	deps.overlay = outcome
	deps.cancel = func() {
		restarted.Store(true)
		cancel()
	}
	done := make(chan error, 1)
	go func() { done <- startCRDWatcher(ctx, deps) }()
	require.Eventually(t, func() bool {
		return restarted.Load() ||
			componentState(deps, "crd-watcher").State == "running"
	}, 15*time.Second, 10*time.Millisecond, "the watcher never started")
	cancel()
	require.NoError(t, <-done)
	return restarted.Load()
}

// The overlay is applied at bootstrap, the watcher starts only once the
// Lease is held. An edit in between restarts kwatch; without one it
// keeps running.
func TestWatcherAppliesAnEditMadeBetweenBootstrapAndItsStart(t *testing.T) {
	spec := map[string]interface{}{
		"app": map[string]interface{}{"clusterName": "overlay"},
	}
	edit := func(configs dynamic.ResourceInterface) {
		obj, err := configs.Get(context.Background(), "kwatch",
			metav1.GetOptions{})
		require.NoError(t, err)
		obj.Object["spec"] = map[string]interface{}{
			"app": map[string]interface{}{"clusterName": "edited"},
		}
		_, err = configs.Update(context.Background(), obj,
			metav1.UpdateOptions{})
		require.NoError(t, err)
	}
	remove := func(configs dynamic.ResourceInterface) {
		require.NoError(t, configs.Delete(context.Background(), "kwatch",
			metav1.DeleteOptions{}))
	}
	untouched := func(dynamic.ResourceInterface) {}

	require.True(t, watcherAfterBootstrap(t, overlayClient(spec), edit),
		"an edit before the watcher started was not applied")
	require.True(t, watcherAfterBootstrap(t, overlayClient(spec), remove),
		"a delete before the watcher started was not applied")
	require.False(t,
		watcherAfterBootstrap(t, overlayClient(spec), untouched),
		"an unchanged KwatchConfig restarted kwatch")
}
