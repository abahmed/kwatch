package dynamicwatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

const generationStopTimeout = 10 * time.Second

const discoveryTimeout = 10 * time.Second

// ResourceSpec describes one optional dynamic resource and its callbacks.
// Dynamic watcher mechanics stay here; graph packages retain all domain
// meaning in their callbacks.
type ResourceSpec struct {
	GVR        schema.GroupVersionResource
	Namespaced bool
	Handlers   cache.ResourceEventHandlerFuncs
}

// Watcher owns informer factories and synchronization state for optional
// dynamic APIs. A watcher can be inspected without exposing its factories.
type Watcher struct {
	client     dynamic.Interface
	discovery  discovery.DiscoveryInterfaceWithContext
	resync     time.Duration
	namespaces func(bool) []string
	transform  cache.TransformFunc

	mu          sync.RWMutex
	lifecycleMu sync.Mutex
	factories   []dynamicinformer.DynamicSharedInformerFactory
	informers   []cache.SharedIndexInformer
	skippedGVR  []schema.GroupVersionResource
	unavailable map[string]bool
	cancel      context.CancelFunc
	done        chan struct{}
	runWG       *sync.WaitGroup
	started     bool
	generation  uint64
}

// Generation is an immutable view of one watcher start. It prevents a cache
// sync wait for an older generation from accidentally observing a replacement.
type Generation struct {
	watcher   *Watcher
	number    uint64
	informers []cache.SharedIndexInformer
	cancel    context.CancelFunc
	done      <-chan struct{}
}

// Valid reports whether the handle refers to a started watcher generation.
func (g Generation) Valid() bool {
	return g.watcher != nil && g.number != 0
}

// NewWatcher constructs the shared dynamic informer lifecycle. namespaces
// must return the namespace list for the supplied namespaced flag.
func NewWatcher(
	client dynamic.Interface,
	discoveryClient discovery.DiscoveryInterfaceWithContext,
	resync time.Duration,
	namespaces func(bool) []string,
	transform cache.TransformFunc,
) *Watcher {
	return &Watcher{
		client: client, discovery: discoveryClient, resync: resync,
		namespaces: namespaces, transform: transform,
	}
}

// Start discovers, wires, and starts all requested dynamic informers.
func (w *Watcher) Start(
	ctx context.Context,
	specs []ResourceSpec,
) error {
	_, err := w.StartGeneration(ctx, specs)
	return err
}

// StartGeneration starts a watcher and returns the exact lifecycle generation
// created by the call.
func (w *Watcher) StartGeneration(
	ctx context.Context,
	specs []ResourceSpec,
) (Generation, error) {
	if w == nil || w.client == nil {
		return Generation{}, fmt.Errorf("dynamic watcher has no client")
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return Generation{}, nil
	}
	w.mu.Unlock()
	runCtx, cancel := context.WithCancel(ctx)
	started := false
	done := make(chan struct{})
	runWG := &sync.WaitGroup{}
	defer func() {
		if !started {
			cancel()
		}
	}()
	namespaces := w.namespaces
	if namespaces == nil {
		namespaces = func(bool) []string { return []string{""} }
	}

	factories := make([]dynamicinformer.DynamicSharedInformerFactory, 0)
	informers := make([]cache.SharedIndexInformer, 0)
	skipped := make([]schema.GroupVersionResource, 0)
	for _, spec := range specs {
		discoveryCtx, discoveryCancel := context.WithTimeout(
			runCtx, discoveryTimeout,
		)
		available := ResourceAvailableContext(
			discoveryCtx, w.discovery, spec.GVR,
		)
		discoveryCancel()
		if !available {
			skipped = append(skipped, spec.GVR)
			continue
		}
		for _, namespace := range namespaces(spec.Namespaced) {
			factory, informer, err := NewInformer(
				w.client, w.resync, namespace, spec.GVR, w.transform,
			)
			if err != nil {
				return Generation{}, err
			}
			if _, err := informer.AddEventHandler(spec.Handlers); err != nil {
				return Generation{}, fmt.Errorf(
					"register %s informer: %w", spec.GVR, err,
				)
			}
			factories = append(factories, factory)
			informers = append(informers, informer)
		}
	}

	w.mu.Lock()
	unavailableTransitions := w.updateUnavailableLocked(skipped)
	w.factories = factories
	w.informers = informers
	w.skippedGVR = skipped
	w.cancel = cancel
	w.done = done
	w.runWG = runWG
	w.started = true
	w.generation++
	generation := w.generation
	w.mu.Unlock()
	if unavailableTransitions > 0 {
		metrics.DefaultRegistry().OptionalAPIUnavailable.Add(
			int64(unavailableTransitions),
		)
	}
	for _, informer := range informers {
		runWG.Add(1)
		go func(informer cache.SharedIndexInformer) {
			defer runWG.Done()
			informer.Run(runCtx.Done())
		}(informer)
	}
	go w.clearStarted(runCtx, generation, runWG)
	started = true
	return Generation{
		watcher: w, number: generation,
		informers: append([]cache.SharedIndexInformer(nil), informers...),
		cancel:    cancel, done: done,
	}, nil
}

// Replace stops the current watcher generation before starting the supplied
// resources. Replacement is explicit so callers cannot accidentally leave an
// old informer generation running while installing new handlers.
func (w *Watcher) Replace(
	ctx context.Context,
	specs []ResourceSpec,
) error {
	if w == nil {
		return fmt.Errorf("dynamic watcher has no client")
	}
	if err := w.Stop(ctx); err != nil {
		return err
	}
	return w.Start(ctx, specs)
}

// Stop cancels this generation. A stale generation cannot clear state that
// belongs to a newer replacement.
func (g Generation) Stop(ctx context.Context) error {
	if g.watcher == nil {
		return nil
	}
	return g.watcher.stopGeneration(ctx, g)
}

// WaitForCacheSync waits only for informers owned by this generation.
func (g Generation) WaitForCacheSync(ctx context.Context) bool {
	return waitForInformers(ctx, g.informers)
}

// Wait reports whether this generation has stopped before ctx expires. It is
// a lifecycle signal and is safe for repeated callers.
func (g Generation) Wait(ctx context.Context) bool {
	if !g.Valid() || g.done == nil {
		return true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-g.done:
		return true
	case <-ctx.Done():
		return false
	}
}

func (w *Watcher) clearStarted(
	ctx context.Context,
	generation uint64,
	runWG *sync.WaitGroup,
) {
	<-ctx.Done()
	runWG.Wait()
	w.mu.Lock()
	if w.generation == generation {
		w.clearStateLocked()
	}
	w.mu.Unlock()
}

// Stop cancels all informers owned by the watcher. It is safe to call Stop
// more than once and gives callers a clear lifecycle seam for reconfiguration.
func (w *Watcher) Stop(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	generation := Generation{
		watcher: w, number: w.generation,
		informers: append([]cache.SharedIndexInformer(nil), w.informers...),
		cancel:    w.cancel, done: w.done,
	}
	w.mu.Unlock()
	return w.stopGeneration(ctx, generation)
}

func (w *Watcher) stopGeneration(
	ctx context.Context,
	generation Generation,
) error {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	current := w.generation == generation.number && w.started
	cancel := generation.cancel
	done := generation.done
	w.mu.Unlock()
	if !current {
		return nil
	}
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(
			context.Background(), generationStopTimeout,
		)
		defer cancel()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		klog.ErrorS(
			fmt.Errorf("watcher generation did not stop"),
			"dynamic watcher shutdown timed out",
			"generation", generation.number,
		)
		return ctx.Err()
	}
}

func waitForInformers(
	ctx context.Context,
	informers []cache.SharedIndexInformer,
) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	syncFns := make([]cache.InformerSynced, 0, len(informers))
	for _, informer := range informers {
		syncFns = append(syncFns, informer.HasSynced)
	}
	if len(syncFns) == 0 {
		return true
	}
	metrics.DefaultRegistry().WatcherSyncs.Add(1)
	synced := cache.WaitForCacheSync(ctx.Done(), syncFns...)
	if !synced {
		metrics.DefaultRegistry().WatcherSyncFailures.Add(1)
	}
	return synced
}

func (w *Watcher) currentGeneration() Generation {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return Generation{
		watcher: w, number: w.generation,
		informers: append([]cache.SharedIndexInformer(nil), w.informers...),
		cancel:    w.cancel, done: w.done,
	}
}

func (w *Watcher) clearStateLocked() {
	if !w.started {
		return
	}
	w.started = false
	w.cancel = nil
	if w.done != nil {
		close(w.done)
	}
	w.done = nil
	w.runWG = nil
	w.factories = nil
	w.informers = nil
	w.skippedGVR = nil
}

func (w *Watcher) updateUnavailableLocked(
	skipped []schema.GroupVersionResource,
) int {
	current := make(map[string]bool, len(skipped))
	transitions := 0
	for _, gvr := range skipped {
		key := gvr.String()
		current[key] = true
		if !w.unavailable[key] {
			transitions++
		}
	}
	w.unavailable = current
	return transitions
}

// WaitForCacheSync waits for every dynamic informer created by Start.
func (w *Watcher) WaitForCacheSync(ctx context.Context) bool {
	if w == nil {
		return false
	}
	return w.currentGeneration().WaitForCacheSync(ctx)
}
