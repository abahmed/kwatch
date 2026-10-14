package delivery

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// maxUnfinishedSends is how many sends of one job may begin and never
// return (a provider panic, or a crash) before the job is dead-lettered.
// Without it a job that crashes its provider would crash every restart.
const maxUnfinishedSends = 3

// outcomePanicked means the provider panicked during the send.
const outcomePanicked deliverOutcome = -1

// errProviderPanic is the dead-letter reason of a job whose sends kept
// panicking.
var errProviderPanic = errors.New("provider_panic")

// guardedDeliver sends a job and turns a provider panic into a failed
// attempt. The attempt is counted and saved in the outbox before the
// send, so a crash that no recover can catch is counted too. Any send
// that returns takes its count back, so rate-limit retries never add up.
// It returns the job with its attempt count for the next try.
func (m *Manager) guardedDeliver(
	ctx context.Context, entry *providerEntry, job deliverJob,
) (outcome deliverOutcome, next deliverJob) {
	before := job.attempts
	job.attempts++
	box := m.outbox.Load()
	box.setAttempts(job, entry.lookupName(), job.attempts)
	next = job
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		outcome = outcomePanicked
		metrics.DefaultRegistry().IncPanic("delivery")
		// The panic value can hold message text; log its type only.
		klog.ErrorS(nil, "delivery: provider panicked",
			"component", "delivery", "operation", "send",
			"provider", entry.provider.Name(), "key", job.key(),
			"alertKey", alertKey(job), "panicType", fmt.Sprintf("%T", r))
	}()
	outcome = m.deliver(ctx, entry, job)
	next.attempts = before
	box.setAttempts(next, entry.lookupName(), before)
	return outcome, next
}

// alertKey is the dedup key of an incident job, or "".
func alertKey(job deliverJob) string {
	if job.incident == nil {
		return ""
	}
	return job.incident.DedupKey
}
