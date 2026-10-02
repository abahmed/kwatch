// Package storage is kwatch's persistent state: one bbolt file on the
// data volume with one bucket per data class (incidents, changes,
// baselines, evidence, timeline, audit, fingerprints, state, threads).
//
// In: writes from the pipeline and app through small typed views, for
// example IncidentRecords[T] (latest value per key) or ChangeLog[T]
// (time-ordered entries per entity). Out: the saved state restored on
// the next start.
//
// Only the Lease holder claims the store. A handle is bound to one claim
// epoch: a newer claim abandons older handles in this process, and every
// write, including the compactor's deletes, checks that its epoch is
// still the newest stored one. A file with another schema version, or
// one bbolt cannot read (checked page by page at open), is deleted and
// replaced by a fresh store; no backup is kept. Options.DeferRepair
// opens the file read-only and writes nothing, not even the deletion,
// until Claim. A value that does not decode is skipped and counted. The
// Compactor enforces retention and size caps off the decision loop, on
// logical bytes and on the file size (physical.go); a Mirror saves a
// whole keyed snapshot while writing only what changed.
package storage
