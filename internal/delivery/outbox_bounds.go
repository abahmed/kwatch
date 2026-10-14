package delivery

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// setAttempts rewrites the record of a live job with its send count. A
// record already removed stays removed.
func (o *outbox) setAttempts(job deliverJob, provider string, n int) {
	if o == nil || job.outboxID == "" {
		return
	}
	o.mu.Lock()
	queued, ok := o.live[job.outboxID]
	if ok {
		o.puts[job.outboxID] = OutboxRecord{
			Version: outboxRecordVersion, ID: job.outboxID,
			Provider: provider, Message: job.msg,
			Incident: job.incident, Queued: queued, Attempts: n,
		}
		o.rewritten[job.outboxID] = struct{}{}
	}
	o.mu.Unlock()
	if ok {
		o.signal()
	}
}

// sweepExpired drops the records whose jobs are older than outboxMaxAge.
// The jobs themselves are given up by the worker when it reaches them.
func (o *outbox) sweepExpired(now time.Time) {
	o.mu.Lock()
	for id, queued := range o.live {
		if now.Sub(queued) > outboxMaxAge {
			o.removeLocked(id)
			recordOutboxDrop(id, "expired")
		}
	}
	o.publishDepthLocked()
	o.mu.Unlock()
	o.signal()
}

// enforceBoundLocked drops the oldest records over outboxMaxEntries. The
// job itself stays queued in memory; only its crash protection is lost.
func (o *outbox) enforceBoundLocked() {
	for len(o.live) > outboxMaxEntries {
		oldest := ""
		for id := range o.live {
			if oldest == "" || id < oldest {
				oldest = id
			}
		}
		o.removeLocked(oldest)
		recordOutboxDrop(oldest, "outbox_full")
	}
}

func recordOutboxDrop(id, reason string) {
	metrics.DefaultRegistry().OutboxDropped.Add(1)
	klog.V(2).InfoS("outbox record dropped",
		"component", "delivery", "operation", "outbox",
		"id", id, "reason", reason)
}
