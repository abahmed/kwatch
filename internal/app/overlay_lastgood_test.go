package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/storage"
)

func goodSpec() map[string]interface{} {
	return map[string]interface{}{
		"app": map[string]interface{}{"clusterName": "good"},
		"silences": []interface{}{map[string]interface{}{
			"namespaces": []interface{}{"noisy"}}},
	}
}

func badSpec() map[string]interface{} {
	return map[string]interface{}{
		"namespaces": []interface{}{"apps", "!kube-system"},
	}
}

// stateAt is a claimed state file at path, as a leader session holds it.
func stateAt(t *testing.T, path string) diskState {
	t.Helper()
	store, err := storage.Open(path, storage.Options{
		Now: func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) },
	})
	require.NoError(t, err)
	_, err = store.Claim()
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return diskState{store: store}
}

func startOverlay(
	t *testing.T, spec map[string]interface{},
	lastGood func() ([]byte, bool),
) (*config.Config, overlayOutcome) {
	t.Helper()
	t.Setenv("POD_NAMESPACE", "kwatch")
	cfg, outcome, err := applyStartupCRD(context.Background(), crdBase(),
		overlayClient(spec), func() (*config.Config, error) {
			return crdBase(), nil
		}, lastGood)
	require.NoError(t, err)
	return cfg, outcome
}

// keptAt is what the next process would read from the state file.
func keptAt(path string) func() ([]byte, bool) {
	return func() ([]byte, bool) {
		return readLastGood(path, time.Now)
	}
}

// remember runs a leader session's bookkeeping for outcome on the state
// file at path, then closes the file as the pod's end does.
func remember(t *testing.T, path string, outcome overlayOutcome) {
	t.Helper()
	store, err := storage.Open(path, storage.Options{Now: time.Now})
	require.NoError(t, err)
	_, err = store.Claim()
	require.NoError(t, err)
	rememberOverlay(&serverDeps{overlay: outcome}, diskState{store: store})
	require.NoError(t, store.Close())
}

// An invalid KwatchConfig used to drop the whole overlay at the next
// restart: silences and filters vanished. The last spec that applied
// cleanly stands in for it.
func TestStartupFallsBackToTheLastGoodKwatchConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	cfg, outcome := startOverlay(t, goodSpec(), noLastGood)
	require.False(t, outcome.invalid)
	require.Equal(t, "good", cfg.App.ClusterName)
	remember(t, path, outcome)

	cfg, outcome = startOverlay(t, badSpec(), keptAt(path))

	require.True(t, outcome.invalid)
	require.True(t, outcome.lastGood)
	require.Equal(t, "good", cfg.App.ClusterName,
		"the last good spec is applied")
	require.Len(t, cfg.Silences, 1, "its silences are not lost")
}

func TestStartupWithoutALastGoodSpecKeepsTheMountedConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	cfg, outcome := startOverlay(t, badSpec(), keptAt(path))

	require.True(t, outcome.invalid)
	require.False(t, outcome.lastGood)
	require.Equal(t, "base", cfg.App.ClusterName)
	require.NotEmpty(t, outcome.rejected)
}

func TestStartupIgnoresALastGoodSpecThatNowFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	disk := stateAt(t, path)
	require.NoError(t, disk.put(stateOverlayLastGood,
		`{"license":{"key":"KW1-x"}}`))
	require.NoError(t, disk.store.Close())

	cfg, outcome := startOverlay(t, badSpec(), keptAt(path))

	require.True(t, outcome.invalid)
	require.False(t, outcome.lastGood, "today's rules refuse it")
	require.Equal(t, "base", cfg.App.ClusterName)
	require.Empty(t, cfg.License.Key, "nothing of the refused spec stuck")
}

func TestRefusedSpecNeverReplacesTheKeptOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	_, good := startOverlay(t, goodSpec(), noLastGood)
	remember(t, path, good)
	_, refused := startOverlay(t, badSpec(), keptAt(path))
	remember(t, path, refused)

	kept, ok := keptAt(path)()

	require.True(t, ok)
	require.JSONEq(t, string(good.applied), string(kept))
}

// Deleting the KwatchConfig is a choice: a later invalid one must not
// bring the old spec back.
func TestDeletingTheKwatchConfigClearsTheLastGoodSpec(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	_, good := startOverlay(t, goodSpec(), noLastGood)
	remember(t, path, good)
	_, ok := keptAt(path)()
	require.True(t, ok)

	t.Setenv("POD_NAMESPACE", "kwatch")
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			kwatchConfigResource: "KwatchConfigList",
		})
	_, none, err := applyStartupCRD(context.Background(), crdBase(),
		client, loadConfig, noLastGood)
	require.NoError(t, err)
	require.False(t, none.invalid)
	require.Empty(t, none.applied)
	remember(t, path, none)

	_, ok = keptAt(path)()
	require.False(t, ok, "the kept spec is gone")
}

// The file is read without claiming it: a missing or unreadable state
// file reads as no spec, and nothing is created.
func TestReadLastGoodFailsClosed(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "none.db")

	_, ok := readLastGood(missing, time.Now)

	require.False(t, ok)
	require.NoFileExists(t, missing, "reading must not create the file")
}
