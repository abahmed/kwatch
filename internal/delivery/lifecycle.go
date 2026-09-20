package delivery

import (
	"context"
	"fmt"
	"time"
)

// Start launches a worker goroutine for each provider that processes
// queued deliveries. Workers drain and stop when ctx is cancelled.

func (a *Manager) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Lock()
	if a.generationStuck && a.workerCount > 0 {
		a.mu.Unlock()
		return fmt.Errorf("previous delivery generation is still stopping")
	}
	if a.started && !a.stopped {
		a.mu.Unlock()
		return nil
	}
	if a.generation == nil {
		a.generation = a.currentGenerationLocked()
	}
	if a.generation == nil {
		a.generation = newProviderGeneration(nil)
	}
	a.generation.state = generationAccepting
	// Start always owns a fresh set of channels. This also makes test and
	// composition generations that were built without channels safe to run.
	a.generation = cloneProviderGeneration(a.generation, true)
	a.started = true
	a.stopped = false
	a.ctx = ctx
	generation := cloneProviderGeneration(a.generation, false)
	a.workerCtx, a.cancelWorker = context.WithCancel(ctx)
	a.workerDone = make(chan struct{})
	a.workerCount = len(generation.order)
	if a.workerCount == 0 {
		close(a.workerDone)
	}
	a.done = a.workerDone
	for _, name := range generation.order {
		entry := generation.entries[name]
		go a.runProvider(entry, a.workerCtx)
	}
	for _, job := range a.pending {
		a.fanOut(job)
	}
	a.pending = nil
	a.mu.Unlock()
	a.touchProgress()
	return nil
}

// HasProviders reports whether delivery has active provider workers. An empty
// provider set is valid and waits for shutdown rather than failing startup.
func (a *Manager) HasProviders() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.generation != nil && len(a.generation.order) > 0
}

func (a *Manager) runProvider(entry providerEntry, ctx context.Context) {
	defer a.workerFinished()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.touchProgress()
		case job, ok := <-entry.ch:
			if !ok {
				return
			}
			a.touchProgress()
			// Routing is decided before pacing. A job this provider does not
			// want is not a delivery, and making it wait its turn spent the
			// provider's send slot on nothing: with a route that matches one
			// namespace, a storm elsewhere throttled the alerts that did match.
			if job.kind == jobIncident && !shouldDeliver(entry.routes, job.inc) {
				continue
			}
			// Pace before delivering: the queue absorbs the burst, the provider
			// sees a rate it tolerates. Cancellation stops delivery promptly.
			if a.waitForSendSlot(ctx, entry.provider.Name()) {
				a.deliverOne(ctx, &entry, job)
				a.flushDigest(ctx, &entry)
				a.touchProgress()
			}
		}
	}
}

func (a *Manager) workerFinished() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.workerCount == 0 {
		return
	}
	a.workerCount--
	if a.workerCount == 0 && a.workerDone != nil {
		a.generationStuck = false
		close(a.workerDone)
	}
}

// shutdown waits for all delivery workers to finish (used in tests).

func (a *Manager) shutdown() {
	_ = a.shutdownContext(context.Background())
}

func (a *Manager) shutdownContext(ctx context.Context) error {
	a.mu.Lock()
	if a.stopped {
		done := a.workerDone
		cancel := a.cancelWorker
		a.mu.Unlock()
		return waitForWorkers(ctx, done, cancel)
	}
	a.stopped = true
	if a.generation != nil {
		a.generation.state = generationStopped
	}
	generation := cloneProviderGeneration(a.generation, false)
	done := a.workerDone
	cancel := a.cancelWorker
	a.mu.Unlock()

	// Anything still queued when we get here is delivered by the per-provider
	// worker before it returns, but a process killed
	// mid-shutdown loses the queue. That now includes plain messages and
	// events, which used to be sent synchronously by their caller: the
	// startup notification can be lost if kwatch is killed within the pacing
	// delay of starting. Accepted -- the alternative is holding informer
	// startup behind a slow provider, which is what the synchronous path did.
	//
	// 1) close provider channels under a.mu so fanOut (also under a.mu) never
	//    sends on a closed channel.
	a.mu.Lock()
	if generation != nil {
		for _, name := range generation.order {
			if ch := generation.entries[name].ch; ch != nil {
				close(ch)
			}
		}
	}
	a.mu.Unlock()
	if done == nil {
		return nil
	}
	err := waitForWorkers(ctx, done, cancel)
	if err != nil {
		a.mu.Lock()
		a.generationStuck = true
		if a.generation != nil {
			a.generation.state = generationFailed
		}
		a.mu.Unlock()
		a.drainQueuedJobs(generation, "delivery_shutdown")
	}
	return err
}

// drainQueuedJobs records jobs that remained after cancellation or a bounded
// shutdown timeout. Workers may have consumed some jobs concurrently; only
// jobs still in the closed channels are recorded here.
func (a *Manager) drainQueuedJobs(
	generation *providerGeneration,
	reason string,
) {
	if generation == nil {
		return
	}
	for _, name := range generation.order {
		entry := generation.entries[name]
		if entry.ch == nil {
			continue
		}
		for {
			select {
			case job, ok := <-entry.ch:
				if !ok {
					break
				}
				a.recordDeadLetter(&entry, job, fmt.Errorf("%s", reason))
			default:
				break
			}
			if len(entry.ch) == 0 {
				break
			}
		}
	}
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

// Stop stops all provider workers and waits for the current generation to
// drain. The application owns this call, which keeps delivery lifecycle
// visible to the application supervisor instead of hiding a context watcher
// inside the manager.
func (a *Manager) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return a.shutdownContext(ctx)
}

func cloneProviderGeneration(
	generation *providerGeneration,
	newChannels bool,
) *providerGeneration {
	if generation == nil {
		return nil
	}
	clone := &providerGeneration{
		entries: make(map[string]providerEntry, len(generation.entries)),
		order:   append([]string(nil), generation.order...),
		state:   generation.state,
	}
	for name, entry := range generation.entries {
		if newChannels {
			entry.ch = make(chan deliverJob, channelCap)
		}
		clone.entries[name] = entry
	}
	return clone
}

func generationEntries(generation *providerGeneration) []providerEntry {
	if generation == nil {
		return nil
	}
	entries := make([]providerEntry, 0, len(generation.order))
	for _, name := range generation.order {
		entries = append(entries, generation.entries[name])
	}
	return entries
}

// Done returns a channel that is closed when the Manager has fully
// drained and shut down (all provider workers finished).

func (a *Manager) Done() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.done != nil {
		return a.done
	}
	ch := make(chan struct{})
	close(ch)
	return ch
}

// DeadLetters returns a copy of the dead-letter ring buffer.

func (a *Manager) DeadLetters() []DeadLetterEntry {
	a.dlqMu.Lock()
	defer a.dlqMu.Unlock()
	n := a.dlqCount
	out := make([]DeadLetterEntry, n)
	for i := 0; i < n; i++ {
		idx := (a.dlqHead - n + i + dlqCap) % dlqCap
		out[i] = a.dlqRing[idx]
	}
	return out
}
