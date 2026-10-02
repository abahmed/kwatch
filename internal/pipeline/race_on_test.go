//go:build race

package pipeline

// raceEnabled reports whether the test binary was built with -race. The
// race detector slows the engine several times over, so wall-clock
// budgets measured under it say nothing about production speed.
const raceEnabled = true
