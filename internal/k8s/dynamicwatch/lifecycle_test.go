package dynamicwatch

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestWatcherStartIsIdempotentAndStopIsSafe(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			{
				Group: "example.kwatch.dev", Version: "v1", Resource: "widgets",
			}: "WidgetList",
		},
	)
	watcher := NewWatcher(
		client,
		nil,
		0,
		func(bool) []string { return []string{""} },
		nil,
	)
	specs := []ResourceSpec{{
		GVR: schema.GroupVersionResource{
			Group: "example.kwatch.dev", Version: "v1", Resource: "widgets",
		},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, watcher.Start(ctx, specs))
	require.NoError(t, watcher.Start(ctx, specs))
	if got := watcher.Status().Generation; got != 1 {
		t.Fatalf("Status().Generation = %d, want 1", got)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- watcher.Start(ctx, specs)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	watcher.Stop()
	watcher.Stop()
	if got := watcher.Status(); got.State != "unavailable" {
		t.Fatalf("Status after Stop = %+v, want unavailable", got)
	}
}

func TestWatcherReplaceStopsPreviousGeneration(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			{
				Group: "example.kwatch.dev", Version: "v1", Resource: "widgets",
			}: "WidgetList",
		},
	)
	watcher := NewWatcher(
		client, nil, 0, func(bool) []string { return []string{""} }, nil,
	)
	specs := []ResourceSpec{{GVR: schema.GroupVersionResource{
		Group: "example.kwatch.dev", Version: "v1", Resource: "widgets",
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, watcher.Start(ctx, specs))
	require.NoError(t, watcher.Replace(ctx, specs))
	require.Equal(t, 1, watcher.Status().InformerCount)
	watcher.Stop()
}

func TestStaleGenerationCannotStopReplacement(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			{
				Group: "example.kwatch.dev", Version: "v1", Resource: "widgets",
			}: "WidgetList",
		},
	)
	watcher := NewWatcher(
		client, nil, 0, func(bool) []string { return []string{""} }, nil,
	)
	specs := []ResourceSpec{{GVR: schema.GroupVersionResource{
		Group: "example.kwatch.dev", Version: "v1", Resource: "widgets",
	}}}
	ctx := context.Background()
	first, err := watcher.StartGeneration(ctx, specs)
	require.NoError(t, err)
	first.Stop()
	second, err := watcher.StartGeneration(ctx, specs)
	require.NoError(t, err)
	require.True(t, second.Valid())

	first.Stop()
	if got := watcher.Status(); got.State == "unavailable" {
		t.Fatalf("stale generation stopped replacement: %+v", got)
	}
	second.Stop()
}
