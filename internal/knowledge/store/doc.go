// Package store is Kwatch's persistent state: one bbolt file on the
// kwatch data volume. It is the single source of truth for decisions,
// change history, problem history, baselines and evidence.
//
// Exactly one writer exists at a time. The writer claims the store with
// its Lease epoch; every write transaction verifies the epoch first, so a
// pod that lost the Lease can never overwrite newer state.
package store
