package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComponentSupervisorRequiresClock(t *testing.T) {
	require.Panics(t, func() { newComponentSupervisor(nil) })
}

func TestHandleTerminalOptionalExitReportsShutdownTimeout(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	var observed error
	component := componentSpec{
		name:    "stuck",
		onError: func(err error) { observed = err },
	}

	stop := supervisor.handleTerminalOptionalExit(component,
		fmt.Errorf("%w: late", errComponentShutdown))

	require.True(t, stop, "a component ignoring cancel is never restarted")
	require.ErrorIs(t, observed, errComponentShutdown)
	require.ErrorIs(t, <-supervisor.errCh, errComponentShutdown)
}

func TestHandleTerminalOptionalExitDistinguishesRequired(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)

	optional := supervisor.handleTerminalOptionalExit(
		componentSpec{name: "opt"}, errors.New("failed"))
	required := supervisor.handleTerminalOptionalExit(
		componentSpec{name: "req", required: true}, errors.New("failed"))

	require.False(t, optional, "optional failures retry")
	require.True(t, required)
	require.ErrorContains(t, <-supervisor.errCh, "req: failed")
}

func TestComponentSupervisorReportKeepsFirstFailure(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)

	supervisor.report(nil)
	supervisor.report(errors.New("first"))
	supervisor.report(errors.New("second"))

	require.ErrorContains(t, <-supervisor.errCh, "first")
	require.Empty(t, supervisor.errCh)
}

func TestRunComponentWithoutRunFunctionIsNoop(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)

	err := supervisor.runComponent(context.Background(), componentSpec{})

	require.NoError(t, err)
}

func TestRunComponentCleanStopWithoutProgress(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)

	err := supervisor.runComponent(context.Background(), componentSpec{
		cleanStop: true,
		run:       func(context.Context) error { return nil },
	})

	require.ErrorIs(t, err, errComponentCleanStop)
}

func TestRunComponentWithProgressTreatsReturnAsFailure(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	progress := &testProgress{}
	progress.last.Store(time.Now().UnixNano())
	run := func(context.Context) error { return nil }

	err := supervisor.runComponent(context.Background(), componentSpec{
		progress: progress, run: run,
	})
	clean := supervisor.runComponent(context.Background(), componentSpec{
		progress: progress, run: run, cleanStop: true,
	})

	require.ErrorIs(t, err, errComponentStopped)
	require.ErrorIs(t, clean, errComponentCleanStop)
}

func TestRunComponentStallsWhenNoProgressAfterStartup(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	cancelled := make(chan struct{})

	err := supervisor.runComponent(context.Background(), componentSpec{
		progress:       &testProgress{},
		startupTimeout: time.Millisecond,
		stallTimeout:   40 * time.Millisecond,
		run: func(ctx context.Context) error {
			<-ctx.Done()
			close(cancelled)
			return ctx.Err()
		},
	})

	require.ErrorIs(t, err, errComponentStalled)
	<-cancelled
}

func TestRunComponentStopsWhenParentIsCanceledWithProgress(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	progress := &testProgress{}
	progress.last.Store(time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- supervisor.runComponent(ctx, componentSpec{
			progress: progress,
			run: func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				return ctx.Err()
			},
		})
	}()
	<-started
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
}
