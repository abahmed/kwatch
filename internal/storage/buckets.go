package storage

import "time"

// Bucket names one data class in the state file. Each class has one
// shape: keyed (the latest value per key) or log (time-ordered entries
// per entity). Use the typed constructors below rather than a Bucket
// directly; they pick the right shape for you.
type Bucket string

// Data classes. The comment says what the key is and how long data stays.
const (
	// Incidents: key incident ID, value the incident record. A resolved
	// record expires when its writer says so (resolve time plus 7 days).
	Incidents Bucket = "incidents"
	// Changes: log per object key, 30 days.
	Changes Bucket = "changes"
	// Baselines: key workload or entity, rolling aggregates, no expiry.
	Baselines Bucket = "baselines"
	// Evidence: log per incident ID (log excerpts, termination
	// messages), 30 days and at most DefaultEvidenceCap bytes.
	Evidence Bucket = "evidence"
	// Timeline: log per entity (health transitions, changes, events and
	// decisions), 7 days and at most DefaultTimelineCap bytes.
	Timeline Bucket = "timeline"
	// Audit: log per decision stream, 30 days.
	Audit Bucket = "audit"
	// Fingerprints: key object, value its last seen digest, no expiry.
	Fingerprints Bucket = "fingerprints"
	// State: small lifecycle values (cluster identity, version, session,
	// telemetry and upgrade bookkeeping), no expiry.
	State Bucket = "state"
	// Threads: provider thread IDs, no expiry.
	Threads Bucket = "threads"
	// Outbox: key delivery sequence, value one queued delivery job. The
	// delivery manager bounds it (entry count and age) and removes a job
	// once a provider accepted it, so it has no expiry here.
	Outbox Bucket = "outbox"
)

// shape is how a bucket lays out its keys.
type shape int

const (
	keyed shape = iota
	logged
)

// spec is the fixed description of one bucket.
type spec struct {
	name  Bucket
	shape shape
}

// specs lists every bucket; Open creates each one.
var specs = []spec{
	{Incidents, keyed}, {Changes, logged}, {Baselines, keyed},
	{Evidence, logged}, {Timeline, logged}, {Audit, logged},
	{Fingerprints, keyed}, {State, keyed}, {Threads, keyed},
	{Outbox, keyed},
}

// Retention and size defaults. They are constants
// rather than configuration: no existing setting carries them, and the
// defaults fit the default 2Gi volume budget.
const (
	// DefaultLogRetention keeps changes, timeline, evidence and audit
	// entries for 30 days after the entry time.
	DefaultLogRetention = 30 * 24 * time.Hour
	// DefaultSizeCap is the total logical size (key and value bytes)
	// the compactor keeps the store under: 512 MiB, a quarter of the
	// default 2Gi volume. The file is larger than its logical size
	// (bbolt pages are rarely full and freed pages are not returned);
	// once it passes the cap by a quarter (640 MiB) the next Open or
	// Claim rewrites it. The rewrite copies the live data next to the
	// file, up to 512 MiB more plus a 64 MiB margin, so the peak of
	// about 1.2 GiB stays well under 2Gi.
	DefaultSizeCap int64 = 512 << 20
	// DefaultEvidenceCap bounds evidence, the largest and least
	// essential class, to a quarter of the total cap (128 MiB).
	DefaultEvidenceCap = DefaultSizeCap / 4
	// DefaultTimelineRetention keeps timeline entries for 7 days. The
	// timeline is a write-only trail for post-mortems: what the
	// lifecycle remembers (resolved incidents, baselines, recurrence)
	// is stored in its own buckets and is at most 7 days old. A busy
	// cluster writes a timeline entry every second or two, so 30 days
	// of it filled the state file and made every start check it.
	DefaultTimelineRetention = 7 * 24 * time.Hour
	// DefaultTimelineCap bounds the timeline to 32 MiB, about 100,000
	// entries, however fast a cluster writes. The oldest go first.
	DefaultTimelineCap int64 = 32 << 20
)

// Policy is what the compactor enforces.
type Policy struct {
	// Retention is how long log entries stay, by bucket. A bucket that
	// is absent, keyed, or has zero retention keeps entries.
	Retention map[Bucket]time.Duration
	// EvidenceCap bounds the evidence bucket, oldest first.
	EvidenceCap int64
	// TimelineCap bounds the timeline bucket, oldest first.
	TimelineCap int64
	// SizeCap bounds the whole store. Only evictable data (history log
	// entries) is deleted, oldest first, and it always keeps at least a
	// quarter of the cap; pinned data over the cap is reported as
	// Stats.OverCap. A file larger than SizeCap lowers the target by
	// the excess (see physical.go). Zero means DefaultSizeCap.
	SizeCap int64
	// Batch is the most entries one transaction scans or deletes.
	// Zero means defaultBatch.
	Batch int
}

// DefaultPolicy returns the production retention and caps.
func DefaultPolicy() Policy {
	return Policy{
		Retention: map[Bucket]time.Duration{
			Changes: DefaultLogRetention, Evidence: DefaultLogRetention,
			Timeline: DefaultTimelineRetention, Audit: DefaultLogRetention,
		},
		EvidenceCap: DefaultEvidenceCap,
		TimelineCap: DefaultTimelineCap,
		SizeCap:     DefaultSizeCap,
		Batch:       defaultBatch,
	}
}

func capOrDefault(size int64) int64 {
	if size <= 0 {
		return DefaultSizeCap
	}
	return size
}

// IncidentRecords is the incidents bucket: one record per incident ID.
func IncidentRecords[T any](s *Store) Keyed[T] {
	return Keyed[T]{store: s, bucket: Incidents}
}

// BaselineValues is the baselines bucket: one aggregate per entity.
func BaselineValues[T any](s *Store) Keyed[T] {
	return Keyed[T]{store: s, bucket: Baselines}
}

// FingerprintValues is the fingerprints bucket: one digest per object.
func FingerprintValues[T any](s *Store) Keyed[T] {
	return Keyed[T]{store: s, bucket: Fingerprints}
}

// StateValues is the state bucket: small named lifecycle values.
func StateValues[T any](s *Store) Keyed[T] {
	return Keyed[T]{store: s, bucket: State}
}

// ThreadValues is the threads bucket: provider thread IDs.
func ThreadValues[T any](s *Store) Keyed[T] {
	return Keyed[T]{store: s, bucket: Threads}
}

// OutboxValues is the outbox bucket: delivery jobs not yet accepted.
func OutboxValues[T any](s *Store) Keyed[T] {
	return Keyed[T]{store: s, bucket: Outbox}
}

// ChangeLog is the changes bucket: what changed, per object key.
func ChangeLog[T any](s *Store) Log[T] {
	return Log[T]{store: s, bucket: Changes}
}

// EvidenceLog is the evidence bucket: excerpts, per incident ID.
func EvidenceLog[T any](s *Store) Log[T] {
	return Log[T]{store: s, bucket: Evidence}
}

// TimelineLog is the timeline bucket: what happened, per entity.
func TimelineLog[T any](s *Store) Log[T] {
	return Log[T]{store: s, bucket: Timeline}
}

// AuditLog is the audit bucket: decisions, per stream name.
func AuditLog[T any](s *Store) Log[T] {
	return Log[T]{store: s, bucket: Audit}
}
