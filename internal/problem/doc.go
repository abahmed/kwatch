// Package problem groups signals into problems — one per root cause — and
// decides when a person should hear about them.
//
// Signals attach to the problem of their explained root. A problem is
// announced once it has settled, updated only on a material change,
// resolved only after its root stays healthy for an adaptive hold, and
// collapsed into a single "flapping" problem when it keeps recurring.
package problem
