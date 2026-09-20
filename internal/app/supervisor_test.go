package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type testProgress struct{ last atomic.Int64 }

func (p *testProgress) LastProgress() time.Time {
	value := p.last.Load()
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}

func TestComponentSupervisorReportsRequiredFailure(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	started := make(chan struct{})
	supervisor.startOwned(context.Background(), componentSpec{
		name:     "required-test-component",
		required: true,
		run: func(context.Context) error {
			close(started)
			return errors.New("startup failed")
		},
	})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("required component did not start")
	}
	select {
	case err := <-supervisor.errCh:
		if err == nil || err.Error() != "required-test-component: startup failed" {
			t.Fatalf("unexpected component error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("required component failure was not reported")
	}

	supervisor.wg.Wait()
}

func TestComponentSupervisorMarksOptionalFailureDegraded(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	initialized := make(chan struct{})
	close(initialized)
	started := make(chan struct{})
	var gotError error
	supervisor.startOptional(
		ctx, initialized,
		componentSpec{
			name:    "optional-test-component",
			onError: func(err error) { gotError = err },
			run: func(context.Context) error {
				close(started)
				return errors.New("optional failure")
			},
		},
	)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("optional component did not start")
	}
	cancel()
	supervisor.wg.Wait()

	if gotError == nil || gotError.Error() != "optional failure" {
		t.Fatalf("optional failure callback = %v", gotError)
	}
	select {
	case err := <-supervisor.errCh:
		t.Fatalf("optional failure was reported as fatal: %v", err)
	default:
	}
}

func TestComponentSupervisorReportsStalledRequiredComponent(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	progress := &testProgress{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	supervisor.startOwned(ctx, componentSpec{
		name:           "stalled-required-component",
		required:       true,
		progress:       progress,
		startupTimeout: 20 * time.Millisecond,
		stallTimeout:   20 * time.Millisecond,
		run: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})

	select {
	case err := <-supervisor.errCh:
		if !errors.Is(err, errComponentStalled) {
			t.Fatalf("error = %v, want stalled component", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled component was not reported")
	}
	supervisor.wg.Wait()
}

func TestComponentSupervisorTreatsUnexpectedStopAsFailure(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	supervisor.startOwned(context.Background(), componentSpec{
		name:     "unexpected-stop",
		required: true,
		run:      func(context.Context) error { return nil },
	})

	select {
	case err := <-supervisor.errCh:
		if !errors.Is(err, errComponentStopped) {
			t.Fatalf("error = %v, want unexpected stop", err)
		}
	case <-time.After(time.Second):
		t.Fatal("unexpected stop was not reported")
	}
	supervisor.wg.Wait()
}
