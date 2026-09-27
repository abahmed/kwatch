package app

import (
	"context"
	"testing"
)

func TestFenceOnLeadershipLossRealLossDisablesGate(t *testing.T) {
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	leaderCtx, leaderCancel := context.WithCancel(context.Background())

	deps := &serverDeps{ctx: appCtx, persistenceGate: newPersistenceGate()}
	deps.persistenceGate.enable()

	cancelCalled := false
	leaderCancel()
	fenceOnLeadershipLoss(leaderCtx, appCtx, deps, func() {
		cancelCalled = true
	})

	if deps.persistenceGate.enabled() {
		t.Fatalf("expected persistence gate disabled on real loss")
	}
	if !cancelCalled {
		t.Fatalf("expected cancel to be called on real loss")
	}
}

func TestFenceOnLeadershipLossGracefulShutdownKeepsGate(t *testing.T) {
	appCtx, appCancel := context.WithCancel(context.Background())
	leaderCtx, leaderCancel := context.WithCancel(context.Background())
	defer leaderCancel()

	deps := &serverDeps{ctx: appCtx, persistenceGate: newPersistenceGate()}
	deps.persistenceGate.enable()

	// Graceful shutdown: the application context ends first.
	appCancel()

	cancelCalled := false
	fenceOnLeadershipLoss(leaderCtx, appCtx, deps, func() {
		cancelCalled = true
	})

	if !deps.persistenceGate.enabled() {
		t.Fatalf(
			"expected persistence gate to stay enabled on graceful shutdown",
		)
	}
	if cancelCalled {
		t.Fatalf("did not expect cancel to be called on graceful shutdown")
	}
}
