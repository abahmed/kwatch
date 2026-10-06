package dynamicwatch

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// Each dynamic informer costs a fixed handful of goroutines (reflector,
// resync, handler listener pair, controller): about ten with the HTTP
// watch. 236 kinds therefore hold about 2,400 goroutines for as long as
// they run, which is a fixed cost, not a leak. This pins the per-kind
// count, so a change that makes it grow shows up here.
func TestInformerGoroutinesPerKindAreFixed(t *testing.T) {
	const kinds, maxPerKind = 30, 10
	gvrs := map[schema.GroupVersionResource]string{}
	for i := range kinds {
		gvr := schema.GroupVersionResource{Group: "x.io", Version: "v1",
			Resource: fmt.Sprintf("k%ds", i)}
		gvrs[gvr] = fmt.Sprintf("K%dList", i)
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(
		k8sruntime.NewScheme(), gvrs)
	before := runtime.NumGoroutine()
	stop := make(chan struct{})
	defer close(stop)
	for gvr := range gvrs {
		_, informer, err := NewInformer(client, 300*time.Second, "", gvr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := informer.AddEventHandler(
			cache.ResourceEventHandlerFuncs{}); err != nil {
			t.Fatal(err)
		}
		go informer.Run(stop)
	}
	time.Sleep(500 * time.Millisecond)
	per := float64(runtime.NumGoroutine()-before) / kinds
	t.Logf("goroutines per informer: %.1f", per)
	if per > maxPerKind {
		t.Fatalf("%.1f goroutines per informer, want at most %d",
			per, maxPerKind)
	}
	time.Sleep(500 * time.Millisecond)
	again := float64(runtime.NumGoroutine()-before) / kinds
	if again > per+0.5 {
		t.Fatalf("goroutines grew from %.1f to %.1f per informer", per, again)
	}
}
