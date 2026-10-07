//go:build !race

package scenarios

// raceEnabled reports whether the test binary was built with -race (see
// race_on_test.go).
const raceEnabled = false
