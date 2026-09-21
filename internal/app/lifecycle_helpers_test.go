package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
)

func TestComponentProgressTracksInjectedTime(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	progress := newComponentProgress(now)
	if !progress.LastProgress().Equal(now) {
		t.Fatalf("initial progress = %v, want %v", progress.LastProgress(), now)
	}
	later := now.Add(time.Minute)
	progress.Touch(later)
	if !progress.LastProgress().Equal(later) {
		t.Fatalf("updated progress = %v, want %v", progress.LastProgress(), later)
	}
	var nilProgress *componentProgress
	if !nilProgress.LastProgress().IsZero() {
		t.Fatal("nil progress should report zero time")
	}
}

func TestProgressCheckIntervalHasBoundedMinimum(t *testing.T) {
	if got := progressCheckInterval(time.Millisecond); got !=
		10*time.Millisecond {
		t.Fatalf("small progress interval = %s", got)
	}
	if got := progressCheckInterval(time.Second); got != 250*time.Millisecond {
		t.Fatalf("progress interval = %s", got)
	}
}

func TestPersistenceGateAndWriteChecks(t *testing.T) {
	gate := newPersistenceGate()
	if !gate.enabled() || !writesAllowed(nil) ||
		!writesAllowed(func() bool { return true }) {
		t.Fatal("new persistence gate should allow writes")
	}
	gate.disable()
	if gate.enabled() || writesAllowed(func() bool { return false }) {
		t.Fatal("disabled persistence gate allowed a write")
	}
	gate.enable()
	if !gate.enabled() {
		t.Fatal("enabled persistence gate rejected writes")
	}
	var nilGate *persistenceGate
	if !nilGate.enabled() {
		t.Fatal("nil persistence gate should allow writes")
	}
}

func TestDetachedShutdownContextsRetainValues(t *testing.T) {
	parent := context.WithValue(context.Background(), "key", "value")
	parent, cancel := context.WithCancel(parent)
	cancel()
	shutdown, stop := boundedShutdownContext(parent)
	defer stop()
	if shutdown.Value("key") != "value" {
		t.Fatal("shutdown context lost parent values")
	}
	if shutdown.Err() != nil {
		t.Fatal("shutdown context inherited cancellation")
	}
	final, finalStop := finalWriteContext(parent)
	defer finalStop()
	if final.Value("key") != "value" || final.Err() != nil {
		t.Fatal("final write context was not detached")
	}
}

func TestLeaderIdentityAndLeaseNameUseEnvironment(t *testing.T) {
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "custom-lease")
	t.Setenv("KWATCH_INSTALLATION_ID", "ignored")
	if got := electionLeaseName(); got != "custom-lease" {
		t.Fatalf("explicit lease name = %q", got)
	}
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "")
	if got := electionLeaseName(); got != "ignored-leader" {
		t.Fatalf("installation lease name = %q", got)
	}
	t.Setenv("KWATCH_INSTALLATION_ID", "")
	t.Setenv("POD_NAME", "kwatch-0")
	identity, err := podIdentity()
	if err != nil || identity != "kwatch-0" {
		t.Fatalf("pod identity = %q, %v", identity, err)
	}
}

func TestRunWithProgressTouchesClockAndRunsFunction(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	progress := newComponentProgress(time.Time{})
	deps := &serverDeps{clients: client.ClientSet{
		Clock: clock.Func(func() time.Time {
			return now
		})}}
	runErr := errors.New("run failed")
	if err := runWithProgress(
		context.Background(), deps, progress,
		func(context.Context) error { return runErr },
	); !errors.Is(err, runErr) {
		t.Fatalf("runWithProgress() error = %v", err)
	}
	if !progress.LastProgress().Equal(now) {
		t.Fatalf("progress timestamp = %v, want %v", progress.LastProgress(), now)
	}
}
