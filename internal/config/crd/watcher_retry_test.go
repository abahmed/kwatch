package crd

import (
	"context"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

func newFakeKwatchConfigClient(
	objects ...runtime.Object,
) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			gvr: "KwatchConfigList",
		}, objects...)
}

func kwatchConfigObject(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kwatch.abahmed.dev/v1alpha1",
		"kind":       "KwatchConfig",
	}}
	obj.SetNamespace("default")
	obj.SetName(name)
	obj.SetResourceVersion("1")
	return obj
}

func TestCRDDelaysRetryBacksOffAndCaps(t *testing.T) {
	d := defaultCRDDelays()
	want := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		16 * time.Second, 32 * time.Second, time.Minute, time.Minute,
	}
	for i, expected := range want {
		if got := d.retry(i + 1); got != expected {
			t.Fatalf("retry(%d) = %v, want %v", i+1, got, expected)
		}
	}
}

func TestCheckInstalledCRDReportsStartFailure(t *testing.T) {
	cfg := &config.Config{}
	watcher := NewWithClient(
		config.RuntimeConfigFor(cfg), newFakeKwatchConfigClient(),
		"default", 0, func() {}, nil, nil, nil,
	)
	// No active lifecycle makes the informer refuse to start.
	got := watcher.checkInstalledCRD(
		context.Background(), watcher.dynamicClient)
	if got != crdFailed {
		t.Fatalf("checkInstalledCRD = %v, want crdFailed", got)
	}
}

func TestWaitForCRDRetriesAfterStartFailure(t *testing.T) {
	failures := make(chan struct{}, 16)
	recovered := make(chan struct{}, 1)
	sink := func(err error) {
		if err != nil {
			failures <- struct{}{}
			return
		}
		select {
		case recovered <- struct{}{}:
		default:
		}
	}
	cfg := &config.Config{}
	watcher := NewWithClient(
		config.RuntimeConfigFor(cfg), newFakeKwatchConfigClient(),
		"default", 0, func() {}, sink, nil, nil,
	)
	watcher.delays = crdDelays{
		poll: time.Millisecond, retryBase: time.Millisecond,
		retryMax: 2 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		watcher.waitForCRD(ctx, watcher.dynamicClient)
	}()

	for i := 0; i < 3; i++ {
		waitSignal(t, failures, "start failure")
	}
	// The lifecycle becomes active: the next retry must start the informer.
	wg := &sync.WaitGroup{}
	watcher.mu.Lock()
	watcher.runWG = wg
	watcher.mu.Unlock()
	waitSignal(t, recovered, "recovery")

	cancel()
	waitSignal(t, finished, "waitForCRD return")
	wg.Wait()
}

func waitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestListCRDSeesMoreThanOneObject(t *testing.T) {
	client := newFakeKwatchConfigClient(
		kwatchConfigObject("a"), kwatchConfigObject("b"))
	watcher := NewWithClient(
		config.RuntimeConfigFor(&config.Config{}), client,
		"default", 0, func() {}, nil, nil, nil,
	)
	list, err := watcher.listCRD(context.Background(), client)
	if err != nil || len(list.Items) != 2 {
		t.Fatalf("items = %d, err = %v", len(list.Items), err)
	}
}

func TestWatcherEventHandlerUnwrapsDeleteTombstone(t *testing.T) {
	restarts := 0
	watcher := &Watcher{
		restart: func() { restarts++ },
		seen:    map[string]string{"default/a": "1"},
		ready:   true,
	}
	handler := watcher.eventHandler()

	handler.OnDelete(cache.DeletedFinalStateUnknown{
		Key: "default/a", Obj: kwatchConfigObject("a"),
	})

	if restarts != 1 {
		t.Fatalf("restarts = %d, want 1 after tombstone delete", restarts)
	}
	if _, known := watcher.seen["default/a"]; known {
		t.Fatal("deleted object must be forgotten")
	}
}
