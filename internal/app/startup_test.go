package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func newTestStartupManager(
	state *memoryState,
	alertConfig map[string]map[string]interface{},
	appConfig *config.App,
) *startupManager {
	var cfg config.Config
	if appConfig != nil {
		cfg.App = *appConfig
	}
	cfg.Alert = alertConfig
	return newStartupManagerWithRuntime(
		state, config.RuntimeConfigFor(&cfg), clock.RealClock{},
	)
}

func quietApp() *config.App {
	return &config.App{DisableStartupMessage: true}
}

func TestStartupManagerConstruction(t *testing.T) {
	sm := newTestStartupManager(&memoryState{}, nil, &config.App{})
	assert.NotNil(t, sm)
}

func TestStartupManagerFirstRunInitializesState(t *testing.T) {
	state := &memoryState{}
	sm := newTestStartupManager(state, nil, quietApp())

	assert.NoError(t, startManager(sm))

	assert.True(t, state.initialized)
	assert.NotEmpty(t, state.clusterID)
	assert.Equal(t, "dev", state.version)
}

func TestStartupManagerUpgradeRecordsVersion(t *testing.T) {
	state := &memoryState{
		initialized: true, clusterID: "existing", version: "v0.10.0",
	}
	sm := newTestStartupManager(state, nil, quietApp())

	assert.NoError(t, startManager(sm))

	assert.Equal(t, "existing", state.clusterID)
	assert.Equal(t, "dev", state.version)
}

func TestStartupManagerSameVersionKeepsState(t *testing.T) {
	state := &memoryState{
		initialized: true, clusterID: "same", version: "dev",
	}
	sm := newTestStartupManager(state, nil, quietApp())

	assert.NoError(t, startManager(sm))

	assert.Equal(t, "same", state.clusterID)
	assert.Equal(t, "dev", state.version)
}

func TestStartupManagerWithStartupMessageEnabled(t *testing.T) {
	state := &memoryState{}
	sm := newTestStartupManager(state, nil, &config.App{})

	assert.NoError(t, startManager(sm))
	assert.True(t, state.initialized)
}

// startManager runs Start and keeps only its error for tests that assert
// persisted state rather than the returned decision.
func startManager(sm *startupManager) error {
	_, err := sm.Start(context.Background())
	return err
}
