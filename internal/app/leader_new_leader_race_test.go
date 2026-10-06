package app

import (
	"context"
	"sync"
	"testing"

	"github.com/abahmed/kwatch/internal/metrics"
)

// A late OnNewLeader event must never push a pod that already became
// leader back to the "starting" role.
func TestOnNewLeaderNeverDowngradesLeader(t *testing.T) {
	_ = metrics.DefaultRegistry()
	for i := 0; i < 500; i++ {
		deps := testServerDeps()
		c := &leaderCallbacks{
			parent:         context.Background(),
			deps:           deps,
			identity:       "me",
			cancelElection: func() {},
			activeRunner: func(context.Context, *serverDeps) error {
				return nil
			},
			activeErrors: make(chan error, 1),
			activeDone:   make(chan struct{}),
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); c.onNewLeader("me") }()
		go func() {
			defer wg.Done()
			c.onStartedLeading(context.Background())
		}()
		wg.Wait()
		status := deps.healthServer.LeadershipStatus()
		if status == nil || status.Role != "leader" {
			t.Fatalf("iteration %d: role = %+v, want leader", i, status)
		}
	}
}

// After the election stopped, a late OnNewLeader must not overwrite the
// stopped status either.
func TestOnNewLeaderKeepsStoppedStatus(t *testing.T) {
	deps := testServerDeps()
	c := &leaderCallbacks{
		parent: context.Background(), deps: deps, identity: "me",
		cancelElection: func() {},
		activeRunner: func(context.Context, *serverDeps) error {
			return nil
		},
		activeErrors: make(chan error, 1),
		activeDone:   make(chan struct{}),
	}
	c.onStartedLeading(context.Background())
	c.onStoppedLeading()
	c.onNewLeader("other")
	if got := deps.healthServer.LeadershipStatus().Role; got != "stopped" {
		t.Fatalf("role = %q, want stopped", got)
	}
}
