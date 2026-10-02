// Package rootcause holds what the root-cause engine shares with the
// layers above it: the CauseRecord an incident keeps, the confidence
// levels readers see, plain-word notes for what kwatch cannot see, and
// small graph and text helpers (pods, owners, registries, scheduler
// messages). The engine itself is package explain.
//
// Vocabulary:
//   - explain.Cause: one cause the engine states for some failures.
//   - CauseRecord: the record of one explain.Cause that an incident
//     keeps and messages are written from.
//   - Proof: one piece of evidence a CauseRecord keeps, with a
//     ProofCode and numbers. It is an explain.Contribution as stored;
//     writers word it from its code, never from its text.
package rootcause
