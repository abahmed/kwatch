package alert

import (
	"context"
	"time"

	"k8s.io/klog/v2"
)

// AddProvider appends a provider entry for testing or late registration.

func (a *AlertManager) AddProvider(p Provider) {
	if isNilProvider(p) {
		klog.InfoS("nil alert provider was not added")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		klog.InfoS("alert manager is stopping; provider was not added",
			"provider", p.Name())
		return
	}
	entry := providerEntry{
		provider: p,
		retry: retryConfig{
			maxAttempts: 1,
			delay:       time.Second,
			maxBackoff:  defaultMaxBackoff,
		},
		ch: make(chan deliverJob, channelCap),
	}
	a.entries = append(a.entries, entry)
	if a.started {
		a.providerWg.Add(1)
		go func() {
			defer a.providerWg.Done()
			for job := range entry.ch {
				a.deliverOne(a.ctx, &entry, job.inc, job.action, job.insight)
			}
		}()
	}
}

// Start launches a worker goroutine for each provider that processes
// queued deliveries. Workers drain and stop when ctx is cancelled.

func (a *AlertManager) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Lock()
	if a.started && !a.stopped {
		a.mu.Unlock()
		return
	}
	a.started = true
	a.stopped = false
	a.ctx = ctx
	a.done = make(chan struct{})
	entries := make([]providerEntry, len(a.entries))
	copy(entries, a.entries)
	a.mu.Unlock()

	for i := range entries {
		entry := &entries[i]
		a.providerWg.Add(1)
		go func() {
			defer a.providerWg.Done()
			for job := range entry.ch {
				a.deliverOne(a.ctx, entry, job.inc, job.action, job.insight)
			}
		}()
	}
	go func() {
		<-ctx.Done()
		a.shutdown()
	}()
}

// shutdown waits for all delivery workers to finish (used in tests).

func (a *AlertManager) shutdown() {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	entries := make([]providerEntry, len(a.entries))
	copy(entries, a.entries)
	a.mu.Unlock()

	// 1) close provider channels under a.mu so fanOut (also under a.mu) never
	//    sends on a closed channel.
	a.mu.Lock()
	for i := range entries {
		if entries[i].ch != nil {
			close(entries[i].ch)
		}
	}
	a.mu.Unlock()
	a.providerWg.Wait()
	close(a.done)
}

// Done returns a channel that is closed when the AlertManager has fully
// drained and shut down (all provider workers finished).

func (a *AlertManager) Done() <-chan struct{} {
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

func (a *AlertManager) DeadLetters() interface{} {
	a.dlqMu.Lock()
	defer a.dlqMu.Unlock()
	n := 0
	for i := range a.dlqRing {
		if a.dlqRing[i].Timestamp.IsZero() {
			break
		}
		n++
	}
	out := make([]DeadLetterEntry, n)
	for i := 0; i < n; i++ {
		idx := (a.dlqHead - n + i + dlqCap) % dlqCap
		out[i] = a.dlqRing[idx]
	}
	return out
}
