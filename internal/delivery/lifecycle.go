package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Start launches a worker goroutine for each provider that processes
// queued deliveries. Workers stop when ctx is cancelled and leave queued
// jobs for Stop, which delivers them within its own deadline.
func (m *Manager) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	launched, err := m.launchWorkers(ctx)
	if err != nil || !launched {
		return err
	}
	m.touchProgress()
	return nil
}

// launchWorkers starts one worker per provider of the current generation
// and replays the jobs that waited for them. It reports false when
// workers already run.
func (m *Manager) launchWorkers(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLifecycleChannelsLocked()
	if m.done.closed {
		return false, fmt.Errorf("delivery manager is stopped")
	}
	if m.workers.stillStopping() {
		return false, errGenerationStopping
	}
	if m.state.accepting() {
		return false, nil
	}
	m.prepareGenerationLocked()
	m.setStateLocked(m.state.afterStart())
	m.ctx = ctx
	generation := cloneProviderGeneration(m.generation, false)
	workerCtx := m.workers.launch(ctx, len(generation.order))
	for _, name := range generation.order {
		go m.runProvider(generation.entries[name], workerCtx)
	}
	m.outbox.Load().start(ctx)
	m.replayQueuedLocked()
	return true, nil
}

// prepareGenerationLocked makes the generation accepting. Start always
// owns a fresh set of channels, which also makes test and composition
// generations that were built without channels safe to run.
func (m *Manager) prepareGenerationLocked() {
	if m.generation == nil {
		m.generation = newProviderGeneration(nil)
	}
	m.generation.state = generationAccepting
	m.generation = cloneProviderGeneration(m.generation, true)
}

// replayQueuedLocked queues the jobs an earlier session or generation
// left unsent, in their old order, then the jobs created before delivery
// started. Left-over jobs go through the backlog, so any number of them
// fits; the jobs created before Start fit the queues by construction.
func (m *Manager) replayQueuedLocked() {
	generation := m.currentGenerationLocked()
	if generation == nil {
		return
	}
	leftOver := m.restored
	for _, name := range sortedKeys(m.backlog) {
		leftOver = append(leftOver, m.backlog[name]...)
	}
	m.restored, m.backlog = nil, nil
	for _, job := range leftOver {
		name, ok := m.resolveTargetLocked(generation, job)
		if !ok {
			continue
		}
		job.target, job.generation = name, generation
		m.toBacklogLocked(job)
	}
	for _, name := range generation.order {
		m.refillLocked(generation.entries[name])
	}
	for _, job := range m.pending {
		m.fanOut(job)
	}
	m.pending = nil
}

// HasProviders reports whether delivery has active provider workers. An empty
// provider set is valid and waits for shutdown rather than failing startup.
func (m *Manager) HasProviders() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generation != nil && len(m.generation.order) > 0
}

func (m *Manager) runProvider(entry providerEntry, ctx context.Context) {
	defer m.workerFinished()
	ticker := time.NewTicker(workerProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.touchProgress()
		case job, ok := <-entry.ch:
			if !ok {
				return
			}
			m.touchProgress()
			m.publishQueueDepth(entry)
			if !m.workOne(ctx, &entry, job) {
				return
			}
			m.refillFromBacklog(entry)
			m.maybeFlushOverflowSummary(ctx, &entry)
			m.touchProgress()
		}
	}
}

func (m *Manager) workerFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	// The manager is done after a shutdown, not after the drain of a
	// reconfiguration, whose next generation takes over.
	if m.workers.finishOne() && m.state == stateStopped {
		m.closeManagerDoneLocked()
	}
}

// shutdownRun is what one Stop drains: the generation it stopped and the
// handles of that generation's workers.
type shutdownRun struct {
	generation *providerGeneration
	done       chan struct{}
	cancel     context.CancelFunc
	cancelSend context.CancelFunc
}

func (m *Manager) shutdownContext(ctx context.Context) error {
	run, first := m.beginShutdown()
	if !first {
		return m.finishStoppedShutdown(ctx)
	}
	// A request in flight may finish until the drain deadline, which
	// avoids sending it twice; it is cut off when ctx ends.
	if run.cancelSend != nil {
		stopCut := context.AfterFunc(ctx, run.cancelSend)
		defer func() {
			stopCut()
			run.cancelSend()
		}()
	}

	// Shutdown drains the queue within the caller's deadline:
	//
	// 1) close provider channels under m.mu so fanOut (also under m.mu) never
	//    sends on a closed channel;
	// 2) wait for the workers. A live worker delivers what is left in its
	//    closed channel; a worker whose lifecycle context already ended has
	//    returned and left its queue in place;
	// 3) deliver every job still queued, pacing and retrying with ctx;
	// 4) dead-letter whatever could not be sent before ctx ended.
	//
	// A process killed before the deadline still loses the queue; the
	// deadline is sized to fit the Pod termination grace period.
	m.closeQueues(run.generation)
	if err := waitForWorkers(ctx, run.done, run.cancel); err != nil {
		return m.failShutdown(run.generation, err)
	}
	drainErr := m.deliverQueuedJobs(ctx, run.generation)
	if m.closeDoneUnlessReconfiguring() {
		return drainErr
	}
	return errors.Join(drainErr, m.closeOutbox())
}

// beginShutdown moves the manager to its stopped state and returns what
// the shutdown drains. It reports false when an earlier Stop, or the
// drain of a reconfiguration, already began.
func (m *Manager) beginShutdown() (shutdownRun, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.stopped() {
		return shutdownRun{}, false
	}
	m.setStateLocked(m.state.afterStop())
	if !m.state.reconfiguring() {
		// Jobs still pending were never handed to a worker; a reconfiguration
		// replays them into the next generation, a shutdown loses them.
		m.dropPendingLocked()
	}
	if m.generation != nil {
		m.generation.state = generationStopped
	}
	return shutdownRun{
		generation: cloneProviderGeneration(m.generation, false),
		done:       m.workers.done,
		cancel:     m.workers.cancel,
		cancelSend: m.workers.cancelSend,
	}, true
}

// closeQueues closes the provider channels of a stopped generation.
func (m *Manager) closeQueues(generation *providerGeneration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if generation == nil {
		return
	}
	for _, name := range generation.order {
		if ch := generation.entries[name].ch; ch != nil {
			close(ch)
		}
	}
}

// closeDoneUnlessReconfiguring marks the manager done unless a
// reconfiguration is in progress, which it reports.
func (m *Manager) closeDoneUnlessReconfiguring() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.reconfiguring() {
		return true
	}
	m.closeManagerDoneLocked()
	return false
}

// finishStoppedShutdown handles a second Stop: it only waits for the
// workers of the first one and then marks the manager done.
func (m *Manager) finishStoppedShutdown(ctx context.Context) error {
	done, cancel := m.workerHandles()
	err := waitForWorkers(ctx, done, cancel)
	if err == nil {
		m.closeDoneUnlessReconfiguring()
	}
	return err
}

func (m *Manager) workerHandles() (chan struct{}, context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.workers.done, m.workers.cancel
}

// failShutdown records that the workers did not stop in time, dead-letters
// what is still queued and closes the outbox.
func (m *Manager) failShutdown(
	generation *providerGeneration, err error,
) error {
	m.markWorkersStuck()
	m.drainQueuedJobs(generation)
	return errors.Join(err, m.closeOutbox())
}

func (m *Manager) markWorkersStuck() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workers.stuck = true
	if m.generation != nil {
		m.generation.state = generationFailed
	}
}

// closeOutbox makes the final outbox write once delivery has stopped for
// good. A reconfiguration keeps the outbox running for the next
// generation.
func (m *Manager) closeOutbox() error {
	if m.IsReconfiguring() {
		return nil
	}
	return m.outbox.Load().close(outboxFinalTimeout)
}

func waitForWorkers(
	ctx context.Context,
	done <-chan struct{},
	cancel context.CancelFunc,
) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		if cancel != nil {
			cancel()
		}
		return ctx.Err()
	}
}

// Stop stops all provider workers, delivers every job still queued within
// ctx, and dead-letters what it could not send. The application owns this
// call, which keeps delivery lifecycle visible to the application
// supervisor instead of hiding a context watcher inside the manager.
func (m *Manager) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return m.shutdownContext(ctx)
}

// Done returns a channel that is closed when the Manager has fully
// drained and shut down (all provider workers finished).
func (m *Manager) Done() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLifecycleChannelsLocked()
	return m.done.ch
}
