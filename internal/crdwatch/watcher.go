package crdwatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/metrics"
)

var gvr = schema.GroupVersionResource{
	Group:    "kwatch.abahmed.dev",
	Version:  "v1alpha1",
	Resource: "kwatchconfigs",
}

// Watcher monitors KwatchConfig CRs. Config changes are deliberately not
// applied live: a partially reconfigured controller is harder to reason about
// than a brief, explicit restart.

type Watcher struct {
	runtime             config.RuntimeConfig
	dynamicClient       dynamic.Interface
	namespace           string
	resync              time.Duration
	mu                  sync.Mutex
	lifecycleMu         sync.Mutex
	seen                map[string]string
	ready               bool
	started             bool
	resetting           bool
	generation          uint64
	cancel              context.CancelFunc
	restart             func()
	restartRequested    bool
	statusSink          func(error)
	stateSink           func(Status)
	lastError           string
	optionalUnavailable bool
	done                chan struct{}
	runWG               *sync.WaitGroup
}

// NewWithClient constructs the watcher from the application-owned dynamic
// client.
func NewWithClient(
	runtime config.RuntimeConfig,
	dynamicClient dynamic.Interface,
	namespace string,
	resync time.Duration,
	restart func(),
	statusSink func(error),
	stateSink func(Status),
) *Watcher {
	return &Watcher{
		runtime: runtime, dynamicClient: dynamicClient, namespace: namespace,
		resync: resync, seen: make(map[string]string), restart: restart,
		statusSink: statusSink, stateSink: stateSink,
	}
}

func (w *Watcher) Start(ctx context.Context) error {
	if !w.runtime.Monitors().CRD().Enabled {
		klog.V(4).InfoS("CRD watcher is disabled")
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.started = true
	w.generation++
	generation := w.generation
	w.cancel = cancel
	w.done = make(chan struct{})
	w.runWG = &sync.WaitGroup{}
	runWG := w.runWG
	w.mu.Unlock()
	complete := false
	defer func() {
		if !complete {
			cancel()
			runWG.Wait()
			w.resetLifecycle(generation)
		}
	}()

	dc := w.dynamicClient
	if dc == nil {
		err := fmt.Errorf("crdwatch: dynamic client is not configured")
		w.reportError(err)
		return err
	}

	// Pre-flight: check if the CRD is installed. If it is installed later,
	// keep watching for it instead of requiring a process restart.
	if _, err := w.listCRD(runCtx, dc); err != nil {
		if errors.IsNotFound(err) {
			if w.markOptionalUnavailable() {
				metrics.DefaultRegistry().OptionalAPIUnavailable.Add(1)
			}
			klog.InfoS(
				"CRD kwatchconfigs.kwatch.abahmed.dev not found; " +
					"waiting for installation",
			)
			complete = true
			w.reportError(nil)
			go w.runWaitingGeneration(runCtx, dc, generation)
			return nil
		}
		wrapped := fmt.Errorf("crdwatch: preflight check failed: %w", err)
		w.reportError(wrapped)
		return wrapped
	}
	w.clearOptionalUnavailable()
	if err := w.startInformer(runCtx, dc, false); err != nil {
		w.reportError(err)
		return err
	}
	w.reportError(nil)
	complete = true
	go w.waitForGenerationStop(runCtx, generation)
	return nil
}

func (w *Watcher) markOptionalUnavailable() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.optionalUnavailable {
		return false
	}
	w.optionalUnavailable = true
	return true
}

func (w *Watcher) clearOptionalUnavailable() {
	w.mu.Lock()
	w.optionalUnavailable = false
	w.mu.Unlock()
}

// Stop cancels the KwatchConfig informer or discovery wait. It is safe to
// call more than once and does not alter the late-install policy.
func (w *Watcher) Stop(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	cancel := w.cancel
	generation := w.generation
	done := w.done
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		if ctx == nil {
			var stopCancel context.CancelFunc
			ctx, stopCancel = context.WithTimeout(
				context.Background(), 10*time.Second,
			)
			defer stopCancel()
		}
		select {
		case <-done:
		case <-ctx.Done():
			klog.ErrorS(
				fmt.Errorf("CRD watcher generation did not stop"),
				"CRD watcher shutdown timed out",
				"generation", generation,
			)
			return ctx.Err()
		}
	}
	w.resetLifecycle(generation)
	return nil
}

func (w *Watcher) waitForGenerationStop(
	ctx context.Context,
	generation uint64,
) {
	<-ctx.Done()
	w.mu.Lock()
	runWG := w.runWG
	w.mu.Unlock()
	if runWG != nil {
		runWG.Wait()
	}
	w.resetLifecycle(generation)
}

func (w *Watcher) resetLifecycle(generation uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.generation != generation || !w.started || w.resetting {
		return
	}
	w.resetting = true
	w.started = false
	w.cancel = nil
	done := w.done
	w.done = nil
	w.runWG = nil
	w.resetting = false
	w.ready = false
	w.seen = make(map[string]string)
	w.restartRequested = false
	if done != nil {
		close(done)
	}
}

func (w *Watcher) restartForLateConfig(count int) bool {
	if count == 0 || w.restart == nil {
		return false
	}
	w.requestRestart(
		"KwatchConfig appeared after startup; restarting to apply configuration",
	)
	return true
}

func (w *Watcher) requestRestart(message string) {
	w.mu.Lock()
	if w.restartRequested || w.restart == nil {
		w.mu.Unlock()
		return
	}
	w.restartRequested = true
	restart := w.restart
	w.mu.Unlock()
	klog.InfoS(message)
	restart()
}

func (w *Watcher) seedKnown(objects []interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, obj := range objects {
		if accessor, err := meta.Accessor(obj); err == nil {
			w.seen[accessor.GetNamespace()+"/"+accessor.GetName()] = accessor.GetResourceVersion()
		}
	}
	w.ready = true
}

func (w *Watcher) changed(obj interface{}) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	key := accessor.GetNamespace() + "/" + accessor.GetName()
	version := accessor.GetResourceVersion()
	w.mu.Lock()
	previous, known := w.seen[key]
	if !known {
		w.seen[key] = version
	}
	ready := w.ready
	w.mu.Unlock()
	if !ready || w.restart == nil {
		return
	}
	// An Add event after the initial cache seed is a configuration that was
	// not present when startup configuration was applied. Treat it like an
	// update so the process reloads the new object immediately.
	if known && previous == version {
		return
	}
	w.requestRestart("KwatchConfig changed; restarting to apply configuration")
}

func (w *Watcher) deleted(obj interface{}) {
	accessor, err := meta.Accessor(obj)
	if err != nil || w.restart == nil {
		return
	}
	key := accessor.GetNamespace() + "/" + accessor.GetName()
	w.mu.Lock()
	_, known := w.seen[key]
	delete(w.seen, key)
	ready := w.ready
	w.mu.Unlock()
	if !ready || !known {
		return
	}
	w.requestRestart("KwatchConfig changed; restarting to apply configuration")
}
