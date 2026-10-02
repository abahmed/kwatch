package delivery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// errDeliveryShutdown marks jobs that shutdown could not send in time.
var errDeliveryShutdown = errors.New("delivery_shutdown")

// deliverQueuedJobs sends the jobs left in the closed provider channels
// after the workers returned, one goroutine per provider so a slow
// provider does not spend another's budget. Jobs that cannot be sent
// before ctx ends are deferred; the result is ctx's error when any were.
//
// A shutdown also sends the restored backlog. A reconfiguration leaves
// the backlog for the next generation, which streams it in as usual.
func (m *Manager) deliverQueuedJobs(
	ctx context.Context,
	generation *providerGeneration,
) error {
	if generation == nil {
		return nil
	}
	m.mu.Lock()
	withBacklog := !m.state.reconfiguring()
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, name := range generation.order {
		entry := generation.entries[name]
		handedOff := m.takeHandoff(entry)
		var backlog []deliverJob
		if withBacklog {
			backlog = m.takeBacklog(entry)
		}
		if len(handedOff)+len(backlog) == 0 &&
			(entry.ch == nil || len(entry.ch) == 0) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.deliverQueue(ctx, entry, handedOff, backlog)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("drain delivery queue: %w", err)
	}
	return nil
}

// deliverQueue sends the jobs workers handed off, then the jobs left in
// the provider's closed channel, then the restored backlog.
func (m *Manager) deliverQueue(
	ctx context.Context, entry providerEntry,
	handedOff, backlog []deliverJob,
) {
	defer m.markBusy()()
	for _, job := range handedOff {
		m.drainOne(ctx, &entry, job)
	}
	if entry.ch != nil {
		for job := range entry.ch {
			m.drainOne(ctx, &entry, job)
		}
	}
	for _, job := range backlog {
		m.drainOne(ctx, &entry, job)
	}
	if ctx.Err() == nil {
		m.flushOverflowSummary(ctx, &entry)
	}
}

// drainOne sends one job during Stop. A job that cannot be sent before
// ctx ends is deferred: its outbox record stays for the next session.
func (m *Manager) drainOne(
	ctx context.Context, entry *providerEntry, job deliverJob,
) {
	if !routedTo(entry.routes, job) {
		m.outbox.Load().remove(job.outboxID)
		return
	}
	if ctx.Err() != nil ||
		!m.waitForSendSlot(ctx, entry.provider.Name()) {
		m.deferJob(entry, job, errDeliveryShutdown)
		return
	}
	m.deliverOne(ctx, entry, job)
	m.touchProgress()
}

// drainQueuedJobs defers jobs that remained after a bounded shutdown
// timeout. Workers may still consume some jobs concurrently; only jobs
// still in the closed channels are recorded here.
func (m *Manager) drainQueuedJobs(generation *providerGeneration) {
	if generation == nil {
		return
	}
	m.mu.Lock()
	withBacklog := !m.state.reconfiguring()
	m.mu.Unlock()
	for _, name := range generation.order {
		entry := generation.entries[name]
		jobs := m.takeHandoff(entry)
		if withBacklog {
			jobs = append(jobs, m.takeBacklog(entry)...)
		}
		for _, job := range jobs {
			m.deferJob(&entry, job, errDeliveryShutdown)
		}
		m.drainChannel(entry)
	}
}

// drainChannel defers the routed jobs left in a closed or idle provider
// channel without blocking.
func (m *Manager) drainChannel(entry providerEntry) {
	if entry.ch == nil {
		return
	}
	for {
		select {
		case job, ok := <-entry.ch:
			if !ok {
				return
			}
			if routedTo(entry.routes, job) {
				m.deferJob(&entry, job, errDeliveryShutdown)
			}
		default:
			return
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
