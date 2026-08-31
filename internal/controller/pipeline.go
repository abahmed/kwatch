package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/abahmed/kwatch/internal/metrics"
)

// maxSyncRetries bounds how many times a failing key is requeued. With the
// default exponential rate limiter the retries span a few minutes in total,
// which outlasts any transient apiserver hiccup.
const (
	maxSyncRetries    = 15
	syncRecoveryDelay = 5 * time.Minute
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
	started := time.Now()
	defer func() {
		metrics.Default.ProcessingLatencyMs.Store(time.Since(started).Milliseconds())
		metrics.Default.QueueDepth.Store(int64(p.queue.Len()))
	}()
	metrics.Default.QueueDepth.Store(int64(p.queue.Len()))
	if err := p.syncFn(ctx, key); err != nil {
		// Retry transient errors with backoff first. A still-failing resource
		// then moves to a slow recovery cadence, while permanent errors are
		// forgotten immediately.
		if shouldRetrySyncError(ctx, err) && p.queue.NumRequeues(key) < maxSyncRetries {
			p.queue.AddRateLimited(key)
			utilruntime.HandleError(
				fmt.Errorf(
					"error syncing %s %q: %s, requeuing",
					p.name,
					key,
					err.Error(),
				),
			)
			return true
		}
		retryable := shouldRetrySyncError(ctx, err)
		p.queue.Forget(key)
		reason := "dropping non-retryable error"
		if retryable {
			p.queue.AddAfter(key, syncRecoveryDelay)
			reason = fmt.Sprintf("scheduling recovery retry in %s after %d attempts", syncRecoveryDelay, maxSyncRetries)
		}
		utilruntime.HandleError(
			fmt.Errorf("error syncing %s %q: %s, %s", p.name, key, err.Error(), reason),
		)
		return true
	}

	p.queue.Forget(key)
	return true
}

func shouldRetrySyncError(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	return !apierrors.IsNotFound(err) &&
		!apierrors.IsAlreadyExists(err) &&
		!apierrors.IsBadRequest(err) &&
		!apierrors.IsForbidden(err) &&
		!apierrors.IsInvalid(err) &&
		!apierrors.IsMethodNotSupported(err) &&
		!apierrors.IsNotAcceptable(err) &&
		!apierrors.IsRequestEntityTooLargeError(err) &&
		!apierrors.IsUnauthorized(err)
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
