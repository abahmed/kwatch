package controller

import (
	"context"
	"fmt"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// resourcePipeline bundles the plumbing every watched kind shares: a workqueue
// fed by informer event handlers, the informer HasSynced funcs Run must wait
// for, and the handler method each dequeued key is dispatched to.
type resourcePipeline struct {
	name         string // kind label used in sync-error messages
	queueName    string // workqueue name exposed via metrics
	track        string // ChangeTracker resource label, defaults to name
	startWorkers bool
	queue        workqueue.TypedRateLimitingInterface[string]
	synced       []cache.InformerSynced
	syncFn       func(ctx context.Context, key string) error
}

func newResourcePipeline(name, queueName string) *resourcePipeline {
	return &resourcePipeline{
		name:      name,
		queueName: queueName,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: queueName},
		),
	}
}

func (p *resourcePipeline) trackResource() string {
	if p.track != "" {
		return p.track
	}
	return p.name
}

func (p *resourcePipeline) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	p.queue.Add(key)
}

func (p *resourcePipeline) processNextItem(ctx context.Context) bool {
	key, quit := p.queue.Get()
	if quit {
		return false
	}
	defer p.queue.Done(key)

	if err := p.syncFn(ctx, key); err != nil {
		p.queue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing %s %q: %s, requeuing", p.name, key, err.Error()))
		return true
	}

	p.queue.Forget(key)
	return true
}

func (p *resourcePipeline) worker(ctx context.Context) {
	for p.processNextItem(ctx) {
	}
}

func (p *resourcePipeline) shutdown() {
	if p != nil && p.queue != nil {
		p.queue.ShutDown()
	}
}
