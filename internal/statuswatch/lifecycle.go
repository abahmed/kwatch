package statuswatch

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
	"github.com/abahmed/kwatch/internal/metrics"
)

func (m *Monitor) Start(ctx context.Context) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		cancel()
		return nil
	}
	m.started = true
	m.generation++
	generation := m.generation
	m.ctx = runCtx
	m.cancel = cancel
	m.mu.Unlock()
	complete := false
	defer func() {
		if !complete {
			cancel()
			m.resetLifecycle(generation)
		}
	}()
	apiFactory, apiInformer, err := dynamicwatch.NewInformer(
		m.client, m.resync, "", apiServiceGVR, k8s.TrimManagedFields,
	)
	if err != nil {
		return fmt.Errorf("statuswatch: create APIService informer: %w", err)
	}
	if _, err := apiInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: m.processAPIService,
			UpdateFunc: func(_, obj interface{}) {
				m.processAPIService(obj)
			},
			DeleteFunc: m.resolveAPIService,
		},
	); err != nil {
		return err
	}
	crdFactory, crdInformer, err := dynamicwatch.NewInformer(
		m.client, m.resync, "", crdGVR, k8s.TrimManagedFields,
	)
	if err != nil {
		return fmt.Errorf("statuswatch: create CRD informer: %w", err)
	}
	if _, err := crdInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: m.watchCRD,
			UpdateFunc: func(_, obj interface{}) {
				m.watchCRD(obj)
			},
			DeleteFunc: m.deleteCRD,
		},
	); err != nil {
		return err
	}
	if err := m.startStaticWatcher(runCtx); err != nil {
		klog.ErrorS(err, "statuswatch: start built-in status watchers")
	}
	apiFactory.Start(runCtx.Done())
	crdFactory.Start(runCtx.Done())
	// Bounded, because ctx.Done() alone never fires for a cluster where the
	// CRD API is slow or unreachable. A timeout turns that into an error the
	// caller records.
	syncCtx, syncCancel := context.WithTimeout(runCtx, cacheSyncTimeout)
	defer syncCancel()
	metrics.DefaultRegistry().WatcherSyncs.Add(1)
	if !cache.WaitForCacheSync(
		syncCtx.Done(), apiInformer.HasSynced, crdInformer.HasSynced,
	) {
		metrics.DefaultRegistry().WatcherSyncFailures.Add(1)
		return fmt.Errorf("statuswatch: informer sync failed")
	}
	m.mu.Lock()
	staticGeneration := m.staticGeneration
	m.mu.Unlock()
	if staticGeneration.Valid() &&
		!staticGeneration.WaitForCacheSync(syncCtx) {
		return fmt.Errorf("statuswatch: optional watcher sync failed")
	}
	complete = true
	go m.resetWhenDone(runCtx, generation)
	return nil
}

// Stop cancels all informers owned by the status monitor. It is safe to call
// more than once and is useful when an application rebuilds optional monitors.
func (m *Monitor) Stop() {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.mu.Lock()
	cancel := m.cancel
	generation := m.generation
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.resetLifecycle(generation)
}

func (m *Monitor) resetWhenDone(ctx context.Context, generation uint64) {
	<-ctx.Done()
	m.resetLifecycle(generation)
}

func (m *Monitor) resetLifecycle(generation uint64) {
	var stops []context.CancelFunc
	var staticWatcher *dynamicwatch.Watcher
	m.mu.Lock()
	if m.generation != generation {
		m.mu.Unlock()
		return
	}
	for _, stop := range m.stops {
		stops = append(stops, stop)
	}
	for key := range m.factories {
		delete(m.factories, key)
	}
	for key := range m.stops {
		delete(m.stops, key)
	}
	for key := range m.crdVersions {
		delete(m.crdVersions, key)
	}
	staticWatcher = m.staticWatcher
	m.staticGeneration = dynamicwatch.Generation{}
	m.started = false
	m.ctx = nil
	m.cancel = nil
	m.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
	if staticWatcher != nil {
		staticWatcher.Stop()
	}
}

func (m *Monitor) lifecycleContext() (context.Context, uint64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started || m.ctx == nil {
		return nil, 0, false
	}
	return m.ctx, m.generation, true
}

func (m *Monitor) currentContext() context.Context {
	ctx, _, ok := m.lifecycleContext()
	if !ok {
		return nil
	}
	return ctx
}
