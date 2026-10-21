package crd

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

// A KwatchConfig that existed when the first informer synced is deleted
// while that informer is dead. The rebuilt informer lists nothing and
// sends no delete, so the watcher must notice the missing object itself.
func TestRebuiltInformerReportsConfigDeletedWhileDown(t *testing.T) {
	var restarts atomic.Int32
	restarted := make(chan struct{}, 1)
	dc := newFakeKwatchConfigClient()
	watcher := NewWithClient(
		config.RuntimeConfigFor(&config.Config{}), dc, "default", 0,
		func() {
			restarts.Add(1)
			restarted <- struct{}{}
		}, nil, nil, nil)
	watcher.restartWait = func(context.Context, time.Duration) bool {
		return true
	}
	watcher.seedKnown([]interface{}{kwatchConfigObject("a")})
	wg := &sync.WaitGroup{}
	watcher.mu.Lock()
	watcher.runWG = wg
	watcher.mu.Unlock()
	var runs atomic.Int32
	dead := scriptedInformer{crash: true, runs: &runs}
	rebuild := func() (cache.SharedIndexInformer, error) {
		return watcher.newInformer(dc)
	}

	cancel, done, err := watcher.runInformer(
		context.Background(), dead, rebuild)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		waitSignal(t, done, "informer stop")
		wg.Wait()
	}()

	waitSignal(t, restarted, "restart for the deleted KwatchConfig")
	if got := restarts.Load(); got != 1 {
		t.Errorf("restarts = %d, want 1", got)
	}
}

func TestRebuiltInformerKeepsConfigsItStillHolds(t *testing.T) {
	var restarts atomic.Int32
	dc := newFakeKwatchConfigClient(kwatchConfigObject("a"))
	watcher := NewWithClient(
		config.RuntimeConfigFor(&config.Config{}), dc, "default", 0,
		func() { restarts.Add(1) }, nil, nil, nil)
	watcher.seedKnown([]interface{}{kwatchConfigObject("a")})
	inf, err := watcher.newInformer(dc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go inf.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		t.Fatal("informer did not sync")
	}

	watcher.forgetMissing(inf.GetStore())

	if got := restarts.Load(); got != 0 {
		t.Errorf("restarts = %d, want 0: the config is still there", got)
	}
}
