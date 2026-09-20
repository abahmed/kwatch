package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type baselineSaverRecorder struct {
	called       chan struct{}
	contextError error
	hasDeadline  bool
}

func (r *baselineSaverRecorder) SaveBaseline(
	ctx context.Context,
	_ map[string]map[string]int64,
) error {
	r.contextError = ctx.Err()
	_, r.hasDeadline = ctx.Deadline()
	r.called <- struct{}{}
	return nil
}

func TestBaselineSaverFinalWriteUsesBoundedLiveContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	snapshot := make(chan map[string]map[string]int64, 1)
	progress := make(chan struct{}, 1)
	recorder := &baselineSaverRecorder{called: make(chan struct{}, 1)}
	done := make(chan error, 1)

	go func() {
		done <- startBaselineSaverWithStatus(
			ctx, recorder, snapshot, time.Hour, nil,
			func() bool { return true }, func() { progress <- struct{}{} },
		)
	}()
	snapshot <- map[string]map[string]int64{"apps/api": {"pod": 1}}
	select {
	case <-progress:
	case <-time.After(time.Second):
		t.Fatal("baseline saver did not receive the snapshot")
	}
	cancel()

	select {
	case <-recorder.called:
		require.NoError(t, recorder.contextError)
		require.True(t, recorder.hasDeadline)
	case <-time.After(time.Second):
		t.Fatal("baseline saver did not perform its final write")
	}
	require.NoError(t, <-done)
}
