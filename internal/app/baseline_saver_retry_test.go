package app

import (
	"context"
	"errors"
	"testing"

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
