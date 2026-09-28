// Package signal turns entity state into signals: abnormal conditions such
// as a crash-looping container or a node under memory pressure.
//
// Detectors are small and pure: they read one entity (and, when needed, its
// relations) and return the signals that hold now. The Tracker compares
// successive results and reports which signals were raised, changed or
// cleared, so the problem layer sees transitions, never repeated state.
package signal
