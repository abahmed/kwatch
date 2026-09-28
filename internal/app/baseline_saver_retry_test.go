package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type retryBaselineSaver struct {
	err   error
	calls int
}

func (s *retryBaselineSaver) SaveBaseline(
	context.Context,
	map[string]map[string]int64,
) error {
	s.calls++
	return s.err
}

func TestBaselineSaverRetryRecoversAfterWriteFailure(t *testing.T) {
	saver := &retryBaselineSaver{err: errors.New("temporary")}
	state := baselineSaverState{
		pending: map[string]map[string]int64{"apps/api": {"pod": 1}},
	}
	var reported []error
	report := func(err error) { reported = append(reported, err) }

	state.retry(
		context.Background(), saver, func() bool { return true }, report,
	)
	require.Equal(t, 1, saver.calls)
	require.Len(t, reported, 1)
	require.Error(t, reported[0])
	require.NotNil(t, state.retryC)
	state.retryDelay = 0
	state.retryTimer.Stop()
	saver.err = nil
	state.retry(
		context.Background(), saver, func() bool { return true }, report,
	)
	require.Equal(t, 2, saver.calls)
	require.Nil(t, state.retryC)
	require.NoError(t, reported[1])
}

func TestBaselineSaverRetrySkipsWhenWritesDisabled(t *testing.T) {
	saver := &retryBaselineSaver{}
	state := baselineSaverState{
		pending: map[string]map[string]int64{"apps/api": {"pod": 1}},
	}
	state.retry(
		context.Background(), saver, func() bool { return false }, nil,
	)
	require.Zero(t, saver.calls)
}

func TestBaselineSaverFinishReportsFinalWriteFailure(t *testing.T) {
	saver := &retryBaselineSaver{err: errors.New("forbidden")}
	state := baselineSaverState{
		pending: map[string]map[string]int64{"apps/api": {"pod": 1}},
	}
	var reported error
	state.finish(
		context.Background(), saver, func() bool { return true },
		func(err error) { reported = err },
	)
	require.Equal(t, 1, saver.calls)
	require.EqualError(t, reported, "forbidden")
}

func TestBaselineSaverFlushTimerArmsOnlyOncePerInterval(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	saver := &retryBaselineSaver{}
	snapshots := make(chan map[string]map[string]int64, 100)
	saved := make(chan struct{}, 1)
	done := make(chan error, 1)

	go func() {
		done <- startBaselineSaverWithStatus(
			ctx, saver, snapshots, 50*time.Millisecond, nil,
			func() bool { return true },
			func() {
				select {
				case saved <- struct{}{}:
				default:
				}
			},
		)
	}()

	duration := 150 * time.Millisecond
	deadline := time.Now().Add(duration)
	i := 0
	for time.Now().Before(deadline) {
		snapshots <- map[string]map[string]int64{
			"test/app": {"pod": int64(i)},
		}
		i++
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case <-saved:
	case <-time.After(time.Second):
		t.Fatal("baseline saver did not save despite continuous snapshots")
	}
	require.NoError(t, <-done)
	if saver.calls == 0 {
		t.Fatal("baseline saver did not save at all")
	}
}
