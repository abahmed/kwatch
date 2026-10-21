package crd

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/safego"
)

// scriptedInformer panics in Run when crash is set, else runs until
// stopped and reports itself synced.
type scriptedInformer struct {
	cache.SharedIndexInformer
	crash bool
	runs  *atomic.Int32
}

func (s scriptedInformer) Run(stop <-chan struct{}) {
	s.runs.Add(1)
	if s.crash {
		panic("informer blew up")
	}
	<-stop
}

func (s scriptedInformer) HasSynced() bool { return !s.crash }

// GetStore is an empty cache: the scripted informer holds no objects.
func (s scriptedInformer) GetStore() cache.Store {
	return cache.NewStore(cache.MetaNamespaceKeyFunc)
}

func TestRunInformerRebuildsAfterPanicsAndReportsHealth(t *testing.T) {
	statuses := make(chan error, 8)
	watcher := NewWithClient(
		config.RuntimeConfigFor(&config.Config{}), newFakeKwatchConfigClient(),
		"default", 0, func() {},
		func(err error) { statuses <- err }, nil, nil,
	)
	watcher.restartWait = func(context.Context, time.Duration) bool {
		return true
	}
	wg := &sync.WaitGroup{}
	watcher.mu.Lock()
	watcher.runWG = wg
	watcher.mu.Unlock()

	var runs, rebuilt atomic.Int32
	rebuild := func() (cache.SharedIndexInformer, error) {
		crash := rebuilt.Add(1) < safego.SickAfter
		return scriptedInformer{crash: crash, runs: &runs}, nil
	}
	first := scriptedInformer{crash: true, runs: &runs}
	cancel, done, err := watcher.runInformer(
		context.Background(), first, rebuild)
	if err != nil {
		t.Fatal(err)
	}

	waitStatus := func(wantErr bool) {
		t.Helper()
		select {
		case got := <-statuses:
			if (got != nil) != wantErr {
				t.Fatalf("status error = %v, want error: %v", got, wantErr)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("no status report")
		}
	}
	waitStatus(true)  // three crashes in a row: degraded
	waitStatus(false) // the rebuilt informer synced: healthy again
	if got := runs.Load(); got != int32(safego.SickAfter)+1 {
		t.Fatalf("informer runs = %d, want %d", got, safego.SickAfter+1)
	}

	cancel()
	waitSignal(t, done, "informer stop")
	wg.Wait()
}
