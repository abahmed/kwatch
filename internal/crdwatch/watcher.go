package crdwatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
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
	runtime          config.RuntimeConfig
	dynamicClient    dynamic.Interface
	legacyClient     func() (dynamic.Interface, error)
	namespace        string
	resync           time.Duration
	mu               sync.Mutex
	lifecycleMu      sync.Mutex
	seen             map[string]string
	ready            bool
	started          bool
	generation       uint64
	cancel           context.CancelFunc
	restart          func()
	restartRequested bool
	statusSink       func(error)
	lastError        string
}

// Status describes CRD discovery and informer health without exposing the
// watcher's internal clients or lifecycle handles.
type Status struct {
	State          string `json:"state"`
	WaitingForCRD  bool   `json:"waitingForCRD"`
	Ready          bool   `json:"ready"`
	RestartRequest bool   `json:"restartRequested"`
	LastError      string `json:"lastError,omitempty"`
}

// NewWithClient constructs the watcher from the application-owned dynamic
// client. It is the production constructor; New remains for compatibility
// callers that still provide a REST configuration directly.
func NewWithClient(
	runtime config.RuntimeConfig,
	dynamicClient dynamic.Interface,
	namespace string,
	resync time.Duration,
	restart func(),
) *Watcher {
	return &Watcher{
		runtime: runtime, dynamicClient: dynamicClient, namespace: namespace,
		resync: resync, seen: make(map[string]string), restart: restart,
	}
}

// SetStatusSink connects discovery failures to the application health
// boundary. The sink is optional so compatibility callers remain unaffected.
func (w *Watcher) SetStatusSink(sink func(error)) {
	w.mu.Lock()
	w.statusSink = sink
	w.mu.Unlock()
}

// Status returns the current CRD watcher state for diagnostics.
func (w *Watcher) Status() Status {
	if w == nil {
		return Status{State: "unavailable"}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	state := "stopped"
	if w.started {
		state = "running"
	}
	if w.lastError != "" {
		state = "degraded"
	}
	return Status{
		State:          state,
		WaitingForCRD:  w.started && !w.ready,
		Ready:          w.ready,
		RestartRequest: w.restartRequested,
		LastError:      w.lastError,
	}
}

func (w *Watcher) reportError(err error) {
	w.mu.Lock()
	sink := w.statusSink
	if err == nil {
		w.lastError = ""
	} else {
		w.lastError = safeWatcherReason(err.Error())
	}
	w.mu.Unlock()
	if sink != nil {
		sink(err)
	}
}

func (w *Watcher) Start(ctx context.Context) error {
	if !w.runtime.CrdConfig().Enabled {
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
	w.mu.Unlock()
	complete := false
	defer func() {
		if !complete {
			cancel()
			w.resetLifecycle(generation)
		}
	}()

	dc := w.dynamicClient
	if dc == nil && w.legacyClient != nil {
		var err error
		dc, err = w.legacyClient()
		if err != nil {
			return fmt.Errorf("crdwatch: failed to create dynamic client: %w", err)
		}
	}
	if dc == nil {
		err := fmt.Errorf("crdwatch: dynamic client is not configured")
		w.reportError(err)
		return err
	}

	// Pre-flight: check if the CRD is installed. If it is installed later,
	// keep watching for it instead of requiring a process restart.
	if _, err := dc.Resource(gvr).Namespace(w.namespace).List(
		runCtx, metav1.ListOptions{Limit: 1},
	); err != nil {
		if errors.IsNotFound(err) {
			metrics.DefaultRegistry().OptionalAPIUnavailable.Add(1)
			klog.InfoS(
				"CRD kwatchconfigs.kwatch.abahmed.dev not found; " +
					"waiting for installation",
			)
			complete = true
			w.reportError(nil)
			go w.waitForCRD(runCtx, dc)
			go w.resetWhenDone(runCtx, generation)
			return nil
		}
		wrapped := fmt.Errorf("crdwatch: preflight check failed: %w", err)
		w.reportError(wrapped)
		return wrapped
	}
	if err := w.startInformer(runCtx, dc, false); err != nil {
		w.reportError(err)
		return err
	}
	w.reportError(nil)
	complete = true
	go w.resetWhenDone(runCtx, generation)
	return nil
}

// Stop cancels the KwatchConfig informer or discovery wait. It is safe to
// call more than once and does not alter the late-install policy.
func (w *Watcher) Stop() {
	if w == nil {
		return
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	cancel := w.cancel
	generation := w.generation
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.resetLifecycle(generation)
}

func (w *Watcher) resetWhenDone(
	ctx context.Context,
	generation uint64,
) {
	<-ctx.Done()
	w.resetLifecycle(generation)
}

func (w *Watcher) resetLifecycle(generation uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.generation != generation {
		return
	}
	w.started = false
	w.cancel = nil
	w.ready = false
	w.seen = make(map[string]string)
	w.restartRequested = false
}

func (w *Watcher) waitForCRD(ctx context.Context, dc dynamic.Interface) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			list, err := dc.Resource(gvr).Namespace(w.namespace).List(
				ctx, metav1.ListOptions{Limit: 1},
			)
			if err != nil {
				if !errors.IsNotFound(err) {
					klog.V(2).InfoS("CRD watcher discovery unavailable", "error", err)
					w.reportError(err)
				}
				continue
			}
			if w.restartForLateConfig(len(list.Items)) {
				return
			}
			if err := w.startInformer(ctx, dc, true); err != nil {
				klog.ErrorS(err, "CRD watcher failed to start after installation")
				w.reportError(err)
			} else {
				w.reportError(nil)
			}
			return
		}
	}
}

func (w *Watcher) startInformer(
	ctx context.Context,
	dc dynamic.Interface,
	restartOnInitial bool,
) error {

	factory, inf, err := dynamicwatch.NewInformer(
		dc, w.resync, w.namespace, gvr, k8s.TrimManagedFields,
	)
	if err != nil {
		return fmt.Errorf("crdwatch: create informer: %w", err)
	}

	if _, err := inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.changed,
		UpdateFunc: func(_, obj interface{}) { w.changed(obj) },
		DeleteFunc: w.deleted,
	}); err != nil {
		return fmt.Errorf("crdwatch: failed to register event handler: %w", err)
	}

	factory.Start(ctx.Done())
	metrics.DefaultRegistry().WatcherSyncs.Add(1)
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		metrics.DefaultRegistry().WatcherSyncFailures.Add(1)
		return fmt.Errorf("crdwatch: failed to sync informer cache")
	}

	initial := inf.GetStore().List()
	w.seedKnown(initial)
	if restartOnInitial {
		w.restartForLateConfig(len(initial))
	}

	klog.InfoS("CRD watcher started", "namespace", w.namespace)
	return nil
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
