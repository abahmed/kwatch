//go:build race

package scenarios

// raceEnabled reports whether the test binary was built with -race. The
// race detector multiplies the time and the memory of a replay, so the
// soak test, which measures the heap, runs without it (make
// verify-latency).
const raceEnabled = true
