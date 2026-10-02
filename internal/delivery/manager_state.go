package delivery

import (
	"k8s.io/klog/v2"
)

// managerState is where the Manager is in its lifecycle.
//
// Three base states describe the workers: idle (none started yet), running
// (workers take jobs) and stopped (Stop began). A generation replacement
// passes through the same three steps, so each base state has a
// "reconfigure" twin: the old workers run, then drain, then the new
// generation is published idle and started. The twins behave like their
// base state, except that jobs and leftovers are kept for the next
// generation instead of being dropped.
//
// Every change goes through setStateLocked, which checks it against
// managerTransitions.
type managerState int

const (
	// stateIdle: no workers have started. New jobs wait in the pending
	// queue until Start.
	stateIdle managerState = iota
	// stateRunning: workers take jobs from the provider queues.
	stateRunning
	// stateStopped: Stop began. New jobs are dropped. Done closes once
	// the workers have returned.
	stateStopped
	// stateReconfigureRunning: a generation replacement is in progress
	// and workers still take jobs. The old generation is here until its
	// drain begins; the new one is here from its Start until the
	// replacement is marked finished.
	stateReconfigureRunning
	// stateReconfigureDraining: the old generation is draining. New jobs
	// wait in the pending queue for the next generation.
	stateReconfigureDraining
	// stateReconfigureIdle: the new generation is published and its
	// workers have not started. New jobs wait in the pending queue.
	stateReconfigureIdle
)

// managerTransitions lists, for every state, the states it may move to.
// Each edge changes one thing: workers start, stop, or are replaced, or a
// reconfiguration begins or ends.
var managerTransitions = map[managerState][]managerState{
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

func (s managerState) String() string {
	switch s {
	case stateIdle:
		return "idle"
	case stateRunning:
		return "running"
	case stateStopped:
		return "stopped"
	case stateReconfigureRunning:
		return "reconfigure_running"
	case stateReconfigureDraining:
		return "reconfigure_draining"
	case stateReconfigureIdle:
		return "reconfigure_idle"
	default:
		return "unknown"
	}
}

// canTransition reports whether the lifecycle may move from one state to
// another. Staying in the same state is always allowed.
func canTransition(from, to managerState) bool {
	if from == to {
		return true
	}
	for _, next := range managerTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// setStateLocked moves the lifecycle to a new state. An edge missing from
// managerTransitions is a programming error: it is logged, and the state
// still changes so delivery keeps the behavior its caller asked for. The
// caller holds m.mu.
func (m *Manager) setStateLocked(to managerState) {
	if !canTransition(m.state, to) {
		klog.ErrorS(nil, "invalid delivery state transition",
			"component", "delivery", "operation", "lifecycle",
			"from", m.state.String(), "to", to.String())
	}
	m.state = to
}

// accepting reports whether workers take jobs from the provider queues.
func (s managerState) accepting() bool {
	return s == stateRunning || s == stateReconfigureRunning
}

// stopped reports whether Stop or a reconfiguration drain has begun and
// no new workers have started since.
func (s managerState) stopped() bool {
	return s == stateStopped || s == stateReconfigureDraining
}

// reconfiguring reports whether a generation replacement is in progress.
func (s managerState) reconfiguring() bool {
	return s == stateReconfigureRunning ||
		s == stateReconfigureDraining ||
		s == stateReconfigureIdle
}

// afterStart is the state Start moves to.
func (s managerState) afterStart() managerState {
	if s.reconfiguring() {
		return stateReconfigureRunning
	}
	return stateRunning
}

// afterStop is the state Stop, or a reconfiguration drain, moves to.
func (s managerState) afterStop() managerState {
	if s.reconfiguring() {
		return stateReconfigureDraining
	}
	return stateStopped
}

// afterPublish is the state a newly published generation starts in: no
// workers yet.
func (s managerState) afterPublish() managerState {
	if s.reconfiguring() {
		return stateReconfigureIdle
	}
	return stateIdle
}

// reconfigureBegun is the reconfigure twin of a base state.
func (s managerState) reconfigureBegun() managerState {
	switch s {
	case stateIdle:
		return stateReconfigureIdle
	case stateRunning:
		return stateReconfigureRunning
	case stateStopped:
		return stateReconfigureDraining
	default:
		return s
	}
}

// reconfigureEnded is the base state of a reconfigure twin.
func (s managerState) reconfigureEnded() managerState {
	switch s {
	case stateReconfigureIdle:
		return stateIdle
	case stateReconfigureRunning:
		return stateRunning
	case stateReconfigureDraining:
		return stateStopped
	default:
		return s
	}
}
