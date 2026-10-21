package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A nil model panics on its first read. The prune loop must contain it
// and still stop with its context.
func TestPruneModelSurvivesAPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		pruneModel(ctx, nil, time.Now)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.Fail(t, "pruneModel did not stop with its context")
	}
}
