package crd

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
	// loadBase reloads the mounted configuration so a changed KwatchConfig
	// can be validated before kwatch restarts to apply it. Nil skips the
	// check.
	loadBase func() (*config.Config, error)
	// delays overrides the poll and retry waits; zero uses the defaults.
	delays crdDelays
}

// NewWithClient constructs the watcher from the application-owned dynamic
// client. loadBase returns a fresh copy of the mounted configuration; the
// watcher overlays a changed KwatchConfig on it and only restarts kwatch
// when the result is valid.
func NewWithClient(
	runtime config.RuntimeConfig,
	dynamicClient dynamic.Interface,
	namespace string,
	resync time.Duration,
	restart func(),
	statusSink func(error),
	stateSink func(Status),
	loadBase func() (*config.Config, error),
) *Watcher {
	return &Watcher{
		runtime: runtime, dynamicClient: dynamicClient, namespace: namespace,
		resync: resync, seen: make(map[string]string), restart: restart,
		statusSink: statusSink, stateSink: stateSink, loadBase: loadBase,
	}
}

// watcherRun is the lifecycle state of one watcher generation.
type watcherRun struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
	wg         *sync.WaitGroup
}

// beginRun starts a new generation and reports false when one is already
// running.
func (w *Watcher) beginRun(ctx context.Context) (watcherRun, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return watcherRun{}, false
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.started = true
	w.generation++
	w.cancel = cancel
	w.done = make(chan struct{})
	w.runWG = &sync.WaitGroup{}
	return watcherRun{
		ctx: runCtx, cancel: cancel,
		generation: w.generation, wg: w.runWG,
	}, true
}

// preflightCRD checks whether the CRD is installed. If it is not, the
// watcher keeps waiting for it in the background instead of requiring a
// process restart, and waiting is true.
func (w *Watcher) preflightCRD(
	ctx context.Context, dc dynamic.Interface, generation uint64,
) (waiting bool, err error) {
	if _, err := w.listCRD(ctx, dc); err != nil {
		if !errors.IsNotFound(err) {
			wrapped := fmt.Errorf("crdwatch: preflight check failed: %w", err)
			w.reportError(wrapped)
			return false, wrapped
		}
		if w.markOptionalUnavailable() {
			metrics.DefaultRegistry().OptionalAPIUnavailable.Add(1)
		}
		klog.InfoS(
			"CRD kwatchconfigs.kwatch.abahmed.dev not found; " +
				"waiting for installation",
		)
		w.reportError(nil)
		go w.runWaitingGeneration(ctx, dc, generation)
		return true, nil
	}
	return false, nil
}

// Start watches KwatchConfig resources until ctx ends or a replacement
// generation is needed.
func (w *Watcher) Start(ctx context.Context) error {
	if !w.runtime.Lifecycle().CRDEnabled() {
		klog.V(4).InfoS("CRD watcher is disabled")
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	run, ok := w.beginRun(ctx)
	if !ok {
		return nil
	}
	runCtx, cancel, generation := run.ctx, run.cancel, run.generation
	runWG := run.wg
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

	waiting, err := w.preflightCRD(runCtx, dc, generation)
	if err != nil {
		return err
	}
	if waiting {
		complete = true
		return nil
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

// restartForLateObjects restarts kwatch for KwatchConfig objects that
// appeared after startup, unless one of them is invalid.
func (w *Watcher) restartForLateObjects(objects []interface{}) bool {
	for _, obj := range objects {
		if err := w.validateOverlay(obj, len(objects)); err != nil {
			w.rejectOverlay(obj, err)
			return false
		}
	}
	return w.restartForLateConfig(len(objects))
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
			key := accessor.GetNamespace() + "/" + accessor.GetName()
			w.seen[key] = accessor.GetResourceVersion()
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
	objects := len(w.seen)
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
	if err := w.validateOverlay(obj, objects); err != nil {
		w.rejectOverlay(obj, err)
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
