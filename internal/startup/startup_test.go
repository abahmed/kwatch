package startup

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
) *StartupManager {
	var cfg config.Config
	if appConfig != nil {
		cfg.App = *appConfig
	}
	cfg.Alert = alertConfig
	return NewStartupManagerWithRuntime(
		state, config.RuntimeConfigFor(&cfg), clock.RealClock{},
	)
}

func quietApp() *config.App {
	return &config.App{DisableStartupMessage: true}
}

func TestStartupManagerConstruction(t *testing.T) {
	sm := newTestStartupManager(&memoryState{}, nil, &config.App{})
	assert.NotNil(t, sm)
	assert.NotNil(t, sm.GetPersistenceManager())
}

func TestHandleStartupFirstRunInitializesState(t *testing.T) {
	state := &memoryState{}
	sm := newTestStartupManager(state, nil, quietApp())

	assert.NoError(t, sm.HandleStartup(context.Background()))

	assert.True(t, state.initialized)
	assert.NotEmpty(t, state.clusterID)
	assert.Equal(t, "dev", state.version)
}

func TestHandleStartupUpgradeRecordsVersion(t *testing.T) {
	state := &memoryState{
		initialized: true, clusterID: "existing", version: "v0.10.0",
	}
	sm := newTestStartupManager(state, nil, quietApp())

	assert.NoError(t, sm.HandleStartup(context.Background()))

	assert.Equal(t, "existing", state.clusterID)
	assert.Equal(t, "dev", state.version)
}

func TestHandleStartupSameVersionKeepsState(t *testing.T) {
	state := &memoryState{
		initialized: true, clusterID: "same", version: "dev",
	}
	sm := newTestStartupManager(state, nil, quietApp())

	assert.NoError(t, sm.HandleStartup(context.Background()))

	assert.Equal(t, "same", state.clusterID)
	assert.Equal(t, "dev", state.version)
}

func TestHandleStartupWithStartupMessageEnabled(t *testing.T) {
	state := &memoryState{}
	sm := newTestStartupManager(state, nil, &config.App{})

	assert.NoError(t, sm.HandleStartup(context.Background()))
	assert.True(t, state.initialized)
}
