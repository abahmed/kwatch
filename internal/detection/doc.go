// Package detection finds abnormal conditions (Findings) such as a
// crash-looping container or a node under memory pressure.
// In: entities read from the inventory. Out: Finding transitions. Each
// Detector checks one entity; the Tracker compares successive results
// and reports which findings were raised, changed or cleared.
//
// Vocabulary:
//   - Finding: one abnormal condition of one entity.
//   - Evidence: an observation a detector attaches to its finding, such
//     as an error message. Root-cause scoring has Contributions
//     (explain) and investigation has Facts (incident); Evidence is only
//     ever a detector's.
//   - Mode: a finding's stable failure identity, such as "CrashLoop".
//     Every built-in mode has a constant (mode_names.go); tables match
//     modes through these constants, and KnownMode lets tests reject a
//     misspelt one.
package detection
