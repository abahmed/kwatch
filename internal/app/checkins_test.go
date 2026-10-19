package app

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/license"
)

type fakePinger struct {
	mu     sync.Mutex
	status license.Status
	err    error
	pings  []license.Ping
}

func (f *fakePinger) Status() license.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakePinger) Ping(_ context.Context, p license.Ping) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pings = append(f.pings, p)
	return f.err
}

func (f *fakePinger) sent() []license.Ping {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]license.Ping(nil), f.pings...)
}

func activePro(lid string) license.Status {
	return license.Status{State: license.StateActive,
		Plan: license.PlanPro, LID: lid}
}

func testCheckins(t *testing.T, p *fakePinger) *checkinRunner {
	t.Helper()
	return &checkinRunner{
		pinger: p, disk: diskState{store: openTestStore(t)},
		enabled: true, clusterName: "prod", namespace: "kwatch",
		ready: func() bool { return true },
		// The midpoint keeps every wait at exactly checkinInterval.
		randN: func(n time.Duration) time.Duration { return n / 2 },
	}
}

func TestCheckinRunnerJittersTheInterval(t *testing.T) {
	p := &fakePinger{status: activePro("lic_…1")}
	c := testCheckins(t, p)
	ctx := context.Background()

	c.randN = func(time.Duration) time.Duration { return 0 }
	require.Equal(t, checkinInterval-checkinJitter, c.tick(ctx))
	c.randN = func(n time.Duration) time.Duration { return n - 1 }
	require.Equal(t, checkinInterval+checkinJitter, c.tick(ctx))

	p.err = errors.New("network down")
	require.Equal(t, checkinInterval+checkinJitter, c.tick(ctx),
		"a failed check-in waits a jittered interval too")
}

func TestCheckinRunnerDelaysTheFirstCheckin(t *testing.T) {
	p := &fakePinger{status: activePro("lic_…1")}
	c := testCheckins(t, p)
	c.randN = func(n time.Duration) time.Duration { return n - 1 }
	var first time.Duration
	c.wait = func(_ context.Context, d time.Duration) bool {
		first = d
		return false
	}
	require.NoError(t, c.run(context.Background()))
	require.Equal(t, checkinStartJitter, first)
	require.Empty(t, p.sent())
}

func TestCheckinIntervalStaysWithinBounds(t *testing.T) {
	c := &checkinRunner{randN: rand.N[time.Duration]}
	for range 1000 {
		d := c.interval()
		require.GreaterOrEqual(t, d, checkinInterval-checkinJitter)
		require.LessOrEqual(t, d, checkinInterval+checkinJitter)
	}
}

func TestCheckinRunnerPingsOnlyWhileProIsActive(t *testing.T) {
	p := &fakePinger{status: license.Status{
		State: license.StateCommunity}}
	c := testCheckins(t, p)

	require.Equal(t, checkinInterval, c.tick(context.Background()))
	require.Empty(t, p.sent(), "Community never checks in")

	p.status = activePro("lic_…1")
	c.tick(context.Background())
	require.Equal(t, []license.Ping{{ClusterName: "prod",
		Namespace: "kwatch", Checkins: true}}, p.sent())

	p.status = license.Status{State: license.StateExpired}
	c.tick(context.Background())
	require.Len(t, p.sent(), 1, "an expired license stops check-ins")
}

func TestCheckinRunnerWaitsForReadiness(t *testing.T) {
	p := &fakePinger{status: activePro("lic_…1")}
	c := testCheckins(t, p)
	c.ready = func() bool { return false }
	require.Equal(t, checkinReadyCheck, c.tick(context.Background()))
	require.Empty(t, p.sent())
}

func TestCheckinRunnerStopsUntilTheLicenseChanges(t *testing.T) {
	p := &fakePinger{status: activePro("lic_…1"),
		err: license.ErrRevoked}
	c := testCheckins(t, p)
	c.tick(context.Background())
	c.tick(context.Background())
	require.Len(t, p.sent(), 1, "a refused license is not retried")

	p.status, p.err = activePro("lic_…2"), nil
	c.tick(context.Background())
	require.Len(t, p.sent(), 2, "a new license resumes check-ins")
}

func TestCheckinRunnerHonoursRetryAfter(t *testing.T) {
	p := &fakePinger{status: activePro("lic_…1"),
		err: license.ErrRateLimited{RetryAfter: 45 * time.Minute}}
	c := testCheckins(t, p)
	require.Equal(t, 45*time.Minute, c.tick(context.Background()))
	p.err = &license.TransientError{Status: 503}
	require.Equal(t, checkinInterval, c.tick(context.Background()))
}

func TestCheckinRunnerTurnedOffTellsTheServerOnce(t *testing.T) {
	p := &fakePinger{status: activePro("lic_…1")}
	c := testCheckins(t, p)
	c.announceOff(context.Background())
	require.Empty(t, p.sent(), "never on: nothing to turn off")

	c.tick(context.Background())
	for range 3 {
		// A fresh process each time, over the same state file.
		off := *c
		off.enabled = false
		off.announceOff(context.Background())
	}
	sent := p.sent()
	require.Len(t, sent, 2)
	require.False(t, sent[1].Checkins)
}

func TestCheckinRunnerRunEndsWithContext(t *testing.T) {
	p := &fakePinger{status: activePro("lic_…1")}
	c := testCheckins(t, p)
	ctx, cancel := context.WithCancel(context.Background())
	c.wait = func(context.Context, time.Duration) bool {
		if len(p.sent()) >= 2 {
			cancel()
			return false
		}
		return true
	}
	require.NoError(t, c.run(ctx))
	require.Len(t, p.sent(), 2)
}

func TestCheckinRunnerStoppingSendsTheShutdownState(t *testing.T) {
	p := &fakePinger{status: activePro("lic_…1")}
	c := testCheckins(t, p)
	c.stopping()
	require.Equal(t, []license.Ping{{ClusterName: "prod",
		Namespace: "kwatch", Checkins: true, Stopping: true}}, p.sent())

	var none *checkinRunner
	none.stopping()
	c.enabled = false
	c.stopping()
	require.Len(t, p.sent(), 1, "check-ins off: no shutdown check-in")
}
