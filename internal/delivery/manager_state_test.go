package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var allManagerStates = []managerState{
	stateIdle, stateRunning, stateStopped,
	stateReconfigureRunning, stateReconfigureDraining, stateReconfigureIdle,
}

func TestManagerStateTransitionTable(t *testing.T) {
	allowed := map[managerState][]managerState{
		stateIdle: {
			stateRunning, stateStopped, stateReconfigureIdle,
		},
		stateRunning: {
			stateIdle, stateStopped, stateReconfigureRunning,
		},
		stateStopped: {
			stateIdle, stateRunning, stateReconfigureDraining,
		},
		stateReconfigureRunning: {
			stateRunning, stateReconfigureDraining, stateReconfigureIdle,
		},
		stateReconfigureDraining: {
			stateStopped, stateReconfigureIdle, stateReconfigureRunning,
		},
		stateReconfigureIdle: {
			stateIdle, stateReconfigureRunning, stateReconfigureDraining,
		},
	}
	for _, from := range allManagerStates {
		for _, to := range allManagerStates {
			want := from == to || contains(allowed[from], to)
			assert.Equal(t, want, canTransition(from, to),
				"%s -> %s", from, to)
		}
	}
}

func TestManagerStateStepsFollowTheTable(t *testing.T) {
	steps := map[string]func(managerState) managerState{
		"start":   managerState.afterStart,
		"stop":    managerState.afterStop,
		"publish": managerState.afterPublish,
		"begin":   managerState.reconfigureBegun,
		"end":     managerState.reconfigureEnded,
	}
	for name, step := range steps {
		for _, from := range allManagerStates {
			to := step(from)
			assert.True(t, canTransition(from, to),
				"%s from %s gives %s", name, from, to)
		}
	}
}

func TestManagerStateReconfigureTwinsBehaveLikeTheirBase(t *testing.T) {
	for _, base := range []managerState{
		stateIdle, stateRunning, stateStopped,
	} {
		twin := base.reconfigureBegun()
		assert.True(t, twin.reconfiguring(), "%s", twin)
		assert.False(t, base.reconfiguring(), "%s", base)
		assert.Equal(t, base, twin.reconfigureEnded())
		assert.Equal(t, base.accepting(), twin.accepting(), "%s", base)
		assert.Equal(t, base.stopped(), twin.stopped(), "%s", base)
	}
}

func TestManagerStateLifecycleSequences(t *testing.T) {
	tests := []struct {
		name  string
		steps []func(managerState) managerState
		want  managerState
	}{
		{"start then stop", []func(managerState) managerState{
			managerState.afterStart, managerState.afterStop,
		}, stateStopped},
		{"reconfigure a running manager", []func(managerState) managerState{
			managerState.afterStart, managerState.reconfigureBegun,
			managerState.afterStop, managerState.afterPublish,
			managerState.afterStart, managerState.reconfigureEnded,
		}, stateRunning},
		{"reconfiguration drain fails", []func(managerState) managerState{
			managerState.afterStart, managerState.reconfigureBegun,
			managerState.afterStop, managerState.reconfigureEnded,
		}, stateStopped},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := stateIdle
			for _, step := range tc.steps {
				next := step(state)
				assert.True(t, canTransition(state, next),
					"%s -> %s", state, next)
				state = next
			}
			assert.Equal(t, tc.want, state)
		})
	}
}

func TestManagerSetStateKeepsInvalidTransition(t *testing.T) {
	manager := newTestManager()
	manager.mu.Lock()
	manager.setStateLocked(stateReconfigureDraining)
	manager.mu.Unlock()
	assert.Equal(t, stateReconfigureDraining, manager.state)
}

func contains(states []managerState, state managerState) bool {
	for _, candidate := range states {
		if candidate == state {
			return true
		}
	}
	return false
}
