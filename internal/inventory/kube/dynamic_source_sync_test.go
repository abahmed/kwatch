package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A dynamic kind is synced once its handler has submitted the initial
// list, not as soon as the informer's store holds it.
func TestDynamicSourceKindSyncedWaitsForHandler(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	submit := func(ctx context.Context, obs ...inventory.Observation) {
		for _, o := range obs {
			if o.Entity.Kind != "widget" {
				continue
			}
			select {
			case entered <- struct{}{}:
			default:
				return
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return
		}
	}
	src := NewDynamicSource(DynamicConfig{
		Client:    dynamicClient(widgetCRD(), widget("w")),
		Discovery: newLockedDiscovery(append(anchorLists(), widgetList())...),
		Submit:    submit,
		Now:       func() time.Time { return time.Unix(0, 0) },
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	select {
	case <-entered:
	case <-time.After(testWait):
		t.Fatal("widget handler never ran")
	}

	state, ok := src.KindState("widget")
	require.True(t, ok)
	assert.False(t, state.Synced, "handler has not finished the list")
	assert.Equal(t, ReasonSyncPending, state.Reason)

	close(release)
	require.Eventually(t, func() bool {
		state, _ := src.KindState("widget")
		return state.Synced
	}, testWait, time.Millisecond)
}
