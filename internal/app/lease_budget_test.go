package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
)

func TestDrainBudgetFollowsRemainingLease(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		renewed time.Time
		want    time.Duration
	}{
		{"unknown renewal keeps the default", time.Time{},
			deliveryDrainTimeout},
		{"fresh lease keeps the default", now, deliveryDrainTimeout},
		{"lease close to expiry shrinks the drain",
			now.Add(-20 * time.Second),
			leaderLeaseDuration - 20*time.Second - leaseDrainMargin},
		{"past the renew deadline the lease still holds",
			now.Add(-leaderRenewDeadline - 2*time.Second),
			leaderLeaseDuration - leaderRenewDeadline -
				2*time.Second - leaseDrainMargin},
		{"inside the margin drains nothing",
			now.Add(-leaderLeaseDuration + time.Second), 0},
		{"expired lease never goes negative",
			now.Add(-time.Minute), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := drainBudget(tc.renewed, now, deliveryDrainTimeout)
			require.Equal(t, tc.want, got)
		})
	}
}

// deadlineStopper records the drain context it was given.
type deadlineStopper struct {
	done     bool
	deadline time.Time
	hasLimit bool
}

func (s *deadlineStopper) Stop(ctx context.Context) error {
	s.done = ctx.Err() != nil
	s.deadline, s.hasLimit = ctx.Deadline()
	return ctx.Err()
}

func TestFinishActiveSessionCapsDrainAtLease(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		renewed   time.Time
		wantDone  bool
		wantAtMax time.Duration
	}{
		{"renewed in time drains within the lease",
			now.Add(-20 * time.Second), false,
			leaderLeaseDuration - 20*time.Second - leaseDrainMargin},
		{"stale renewal dead-letters the rest",
			now.Add(-leaderLeaseDuration), true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stopper := &deadlineStopper{}
			s := activeShutdown{
				lastRenewal: func() time.Time { return tc.renewed },
				now:         func() time.Time { return now },
			}
			stopDelivery(context.Background(), stopper, false,
				s.leaseDrainBudget())
			require.Equal(t, tc.wantDone, stopper.done)
			require.True(t, stopper.hasLimit)
			require.LessOrEqual(t, time.Until(stopper.deadline),
				tc.wantAtMax+time.Second)
		})
	}
}

// client-go calls OnStoppedLeading while the session may still be
// draining; the drain budget must still see the last renewal.
func TestStoppedLeadingKeepsLastRenewalForDrainBudget(t *testing.T) {
	deps := testServerDeps()
	renewed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	deps.healthServer.SetLeadership(health.LeadershipStatus{
		Role: "leader", Identity: "kwatch-a",
	})
	deps.healthServer.SetLeadershipRenewal(renewed)
	parent, cancel := context.WithCancel(context.Background())
	cancel() // a graceful shutdown ends the parent first
	callbacks := &leaderCallbacks{
		parent: parent, deps: deps, identity: "kwatch-a",
	}
	callbacks.started.Store(true)

	callbacks.onStoppedLeading()

	require.Equal(t, renewed, lastRenewalFrom(deps)())
	require.Equal(t, "stopped", deps.healthServer.LeadershipStatus().Role)
}

// The acquire write lands before OnStartedLeading, while the role is still
// "starting"; the leader status must still carry that renewal.
func TestStartedLeadingKeepsRenewalFromAcquireWrite(t *testing.T) {
	deps := testServerDeps()
	renewed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	callbacks := &leaderCallbacks{
		parent: context.Background(), deps: deps, identity: "kwatch-a",
		cancelElection: func() {}, activeDone: make(chan struct{}),
		activeErrors: make(chan error, 1),
		activeRunner: func(context.Context, *serverDeps) error {
			return nil
		},
	}
	callbacks.recordRenewal(renewed)

	callbacks.onStartedLeading(context.Background())

	status := deps.healthServer.LeadershipStatus()
	require.Equal(t, "leader", status.Role)
	require.True(t, status.LastRenewal.Equal(renewed))
}
