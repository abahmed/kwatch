// Package flight is kwatch's flight recorder. In: the observation stream
// the sources hand the pipeline (what kwatch replays in tests). Out: a
// bounded, sanitized ring of it in the state file, and an export that
// turns a time window of the ring into a log the scenario library and
// replay can load.
//
// Recording is always on and bounded. Each observation is sanitized when
// it is written (names, namespaces, UIDs, images and label values become
// salted hash pseudonyms; messages and other free text are dropped; a
// Secret keeps only numbers, booleans and times), batched, compressed and
// appended to the flight bucket. The bucket is kept for 48 hours and at
// most 64 MiB; the compactor drops the oldest batches first. Recording
// never delays the pipeline and never blocks it: a slow store drops the
// oldest queued observations and counts them.
//
// The pseudonym salt is random, created once and kept in the state file.
// It is never exported, so an exported log cannot be reversed with it.
//
// Export reads a copy of the state file, never the live one:
//
//	kwatch replay-export --since 2h --out incident.jsonl
//
// This is the only production package that imports internal/replay: it
// uses its log format and sanitizer, so a recording is by construction
// what replay and the scenarios read.
package flight
