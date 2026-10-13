// Package crashtest kills a process in the middle of its writes, for
// tests of what a file holds after a crash.
//
// A test starts its own binary again as a child (Crash). The child opens
// the real store and writes without end, and says "acked n" after write
// n was committed. The parent kills it with SIGKILL, which gives it no
// chance to flush, close or clean up, and then opens the file itself. A
// write that was acked must be there; a write that was not may or may
// not be, but never half of it.
package crashtest
