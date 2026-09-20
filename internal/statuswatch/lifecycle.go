package statuswatch

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	if m.started || m.resetting {
		resetting := m.resetting
		m.mu.Unlock()
		cancel()
		if resetting {
			return fmt.Errorf("statuswatch: previous generation is stopping")
		}
		return nil
	}
	m.started = true
	m.generation++
	generation := m.generation
	m.ctx = runCtx
	m.cancel = cancel
	m.done = make(chan struct{})
	m.runWG = &sync.WaitGroup{}
	m.mu.Unlock()
	complete := false
	defer func() {
		if !complete {
			cancel()
			stopCtx, stopCancel := lifecycleStopContext(nil)
			_ = m.resetLifecycle(stopCtx, generation)
			stopCancel()
		}
	}()
	_, apiInformer, err := dynamicwatch.NewInformer(
		m.client, m.resync, "", apiServiceGVR, k8s.TrimManagedFields,
	)
	if err != nil {
		return fmt.Errorf("statuswatch: create APIService informer: %w", err)
	}
	if _, err := apiInformer.AddEventHandler(k8s.SafeEventHandler(
		"statuswatch", apiServiceGVR.String(),
		cache.ResourceEventHandlerFuncs{
			AddFunc: m.processAPIService,
			UpdateFunc: func(_, obj interface{}) {
				m.processAPIService(obj)
			},
			DeleteFunc: m.resolveAPIService,
		},
	)); err != nil {
		return err
	}
	_, crdInformer, err := dynamicwatch.NewInformer(
		m.client, m.resync, "", crdGVR, k8s.TrimManagedFields,
	)
	if err != nil {
		return fmt.Errorf("statuswatch: create CRD informer: %w", err)
	}
	if _, err := crdInformer.AddEventHandler(k8s.SafeEventHandler(
		"statuswatch", crdGVR.String(),
		cache.ResourceEventHandlerFuncs{
			AddFunc: m.watchCRD,
			UpdateFunc: func(_, obj interface{}) {
				m.watchCRD(obj)
			},
			DeleteFunc: m.deleteCRD,
		},
	)); err != nil {
		return err
	}
	if err := m.startStaticWatcher(runCtx); err != nil {
		klog.ErrorS(err, "statuswatch: start built-in status watchers")
	}
	runWG := m.runWG
	runWG.Add(2)
	go func() {
		defer runWG.Done()
		apiInformer.Run(runCtx.Done())
	}()
	go func() {
		defer runWG.Done()
		crdInformer.Run(runCtx.Done())
	}()
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
func (m *Monitor) Stop(ctx context.Context) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = lifecycleStopContext(nil)
		defer cancel()
	}
	m.mu.Lock()
	cancel := m.cancel
	generation := m.generation
	done := m.done
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if err := m.resetLifecycle(ctx, generation); err != nil {
		return err
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (m *Monitor) resetWhenDone(ctx context.Context, generation uint64) {
	<-ctx.Done()
	stopCtx, stopCancel := lifecycleStopContext(nil)
	defer stopCancel()
	_ = m.resetLifecycle(stopCtx, generation)
}

const lifecycleStopTimeout = 10 * time.Second

func lifecycleStopContext(
	ctx context.Context,
) (context.Context, context.CancelFunc) {
	if ctx != nil {
		return ctx, func() {}
	}
	return context.WithTimeout(context.Background(), lifecycleStopTimeout)
}

func (m *Monitor) resetLifecycle(
	ctx context.Context,
	generation uint64,
) error {
	var stops []context.CancelFunc
	var staticWatcher *dynamicwatch.Watcher
	var done chan struct{}
	var runWG *sync.WaitGroup
	m.mu.Lock()
	if m.generation != generation || !m.started || m.resetting {
		m.mu.Unlock()
		return nil
	}
	m.resetting = true
	for _, stop := range m.stops {
		stops = append(stops, stop)
	}
	staticWatcher = m.staticWatcher
	runWG = m.runWG
	done = m.done
	m.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
	var stopErr error
	if staticWatcher != nil {
		stopErr = staticWatcher.Stop(ctx)
	}
	if runWG != nil {
		waitDone := make(chan struct{})
		go func() {
			runWG.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-ctx.Done():
			if stopErr == nil {
				stopErr = ctx.Err()
			}
		}
	}
	m.mu.Lock()
	if m.generation == generation {
		for key := range m.factories {
			delete(m.factories, key)
		}
		for key := range m.stops {
			delete(m.stops, key)
		}
		for key := range m.versionDone {
			delete(m.versionDone, key)
		}
		for key := range m.crdVersions {
			delete(m.crdVersions, key)
		}
		m.staticGeneration = dynamicwatch.Generation{}
		m.done = nil
		m.started = false
		m.resetting = false
		m.ctx = nil
		m.cancel = nil
		m.runWG = nil
		if done != nil {
			close(done)
		}
	}
	m.mu.Unlock()
	return stopErr
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
