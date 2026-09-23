package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

type changeHistorySaver struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (s *changeHistorySaver) SaveChangeHistory(
	context.Context,
	[]kwcontext.Change,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.err
}

func (s *changeHistorySaver) LoadChangeHistory(
	context.Context,
) ([]kwcontext.Change, error) {
	return nil, nil
}

func (s *changeHistorySaver) callsMade() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestChangeHistorySaverRetriesAfterFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	saver := &changeHistorySaver{err: errors.New("temporary")}
	tracker := kwcontext.NewChangeTrackerWithClock(2, clock.RealClock{})
	reports := make(chan error, 4)

	done := make(chan error, 1)
	go func() {
		done <- startChangeHistorySaverWithInterval(
			ctx, saver, tracker, func(err error) { reports <- err },
			func() bool { return true }, nil, time.Millisecond,
		)
	}()
	select {
	case err := <-reports:
		if err == nil {
			t.Fatal("expected the first history write to fail")
		}
	case <-time.After(time.Second):
		t.Fatal("change history saver did not attempt a write")
	}
	saver.mu.Lock()
	saver.err = nil
	saver.mu.Unlock()
	recovered := false
	deadline := time.After(2 * time.Second)
	for !recovered {
		select {
		case err := <-reports:
			recovered = err == nil
		case <-deadline:
			t.Fatal("change history saver did not recover")
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("saver returned an error: %v", err)
	}
	if saver.callsMade() < 2 {
		t.Fatalf("expected retry, got %d calls", saver.callsMade())
	}
}
