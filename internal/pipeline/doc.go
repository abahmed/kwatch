// Package pipeline runs kwatch's detection flow on one decision loop:
// observations -> inventory -> detection -> rootcause -> incident ->
// compose -> delivery sink.
//
// The loop does no I/O. Each iteration of Engine.Run drains the inbox
// into the model, evaluates the touched entities, solves root cause
// once, ticks the incident lifecycles and hands the decisions to the
// announcer, which writes the messages and calls the sink.
//
// The I/O happens on goroutines the engine owns:
//
//   - The investigation pool (investigationPool) reads logs and the API
//     for newly opened incidents, on a few workers with per-job
//     deadlines. The announcer holds an announcement for its result only
//     for a short wait, then sends it without.
//   - The incident writer (storeWriter) saves incident records,
//     fingerprints and the startup marker, batching the latest snapshot
//     about once a second.
//   - The history writer (historyWriter) saves the timeline and the
//     baselines on the same batching.
//
// Both writers sit behind the engine's persistence, and the loop only
// hands them snapshots. Engine.Run starts every worker and, on shutdown,
// waits for all of them within one bounded deadline.
//
// In: Observations submitted by sources from any goroutine. Out: Messages
// and audit decisions. Only internal/app constructs the Engine.
package pipeline
