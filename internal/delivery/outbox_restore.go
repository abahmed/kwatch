package delivery

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// AttachOutbox loads the jobs an earlier session left unsent and persists
// every job queued from now on. The leader session calls it once, after it
// claimed the state store and before delivery starts. Restored jobs are
// queued ahead of any new work, in the order they were first queued.
func (m *Manager) AttachOutbox(store OutboxStore) error {
	if store == nil {
		return errors.New("delivery outbox store is nil")
	}
	records, err := store.LoadOutbox()
	if err != nil {
		return fmt.Errorf("load delivery outbox: %w", err)
	}
	box := newOutbox(store, m.nowTime)
	jobs := box.adopt(records, m.nowTime())
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.outbox.Load() != nil {
		return errors.New("delivery outbox already attached")
	}
	m.outbox.Store(box)
	m.recordPendingLocked(box)
	if m.state.accepting() {
		box.start(m.ctx)
		m.restored = jobs
		m.replayQueuedLocked()
		return nil
	}
	m.restored = jobs
	return nil
}

// recordPendingLocked writes outbox records for the provider copies held
// before the outbox was attached. The caller holds m.mu.
func (m *Manager) recordPendingLocked(box *outbox) {
	for i, job := range m.pending {
		if job.target != "" && job.outboxID == "" {
			m.pending[i].outboxID = box.add(job, job.target)
		}
	}
}

// adopt turns saved records into queued jobs, oldest first. Records that
// are too old or of an unknown layout are dropped and counted; their
// deletion is written once the writer runs.
func (o *outbox) adopt(records []OutboxRecord, now time.Time) []deliverJob {
	sort.Slice(records, func(i, j int) bool {
		return records[i].ID < records[j].ID
	})
	o.mu.Lock()
	defer o.mu.Unlock()
	jobs := make([]deliverJob, 0, len(records))
	for _, record := range records {
		if seq, err := strconv.ParseUint(record.ID, 10, 64); err == nil &&
			seq > o.seq {
			o.seq = seq
		}
		if reason := rejectRecord(record, now); reason != "" {
			o.removes[record.ID] = struct{}{}
			recordOutboxDrop(record.ID, reason)
			continue
		}
		o.live[record.ID] = record.Queued
		jobs = append(jobs, restoredJob(record))
	}
	before := len(o.live)
	o.enforceBoundLocked()
	if dropped := before - len(o.live); dropped > 0 {
		jobs = jobs[dropped:]
	}
	o.publishDepthLocked()
	return jobs
}

// rejectRecord names why a saved record cannot be sent, or "".
func rejectRecord(record OutboxRecord, now time.Time) string {
	switch {
	case record.Version != outboxRecordVersion:
		return "unknown_version"
	case record.ID == "" || record.Provider == "":
		return "invalid_record"
	case now.Sub(record.Queued) > outboxMaxAge:
		return "expired"
	}
	return ""
}

func restoredJob(record OutboxRecord) deliverJob {
	job := deliverJob{
		kind: jobMessage, msg: record.Message,
		target: record.Provider, outboxID: record.ID,
		queued: record.Queued,
	}
	if record.Incident != nil {
		incident := *record.Incident
		job.kind, job.incident = jobIncident, &incident
	}
	return job
}
