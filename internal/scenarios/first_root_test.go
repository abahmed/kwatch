package scenarios

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/replay"
)

// firstRoot is how long a scenario took to tell people its expected
// root: from the first failure observation (measured as for the first
// message) to the first message that stated that root, on the simulated
// clock. It is reported, not gated: a root that is right in the end but
// late is a quality number, not a failure.
type firstRoot struct {
	scenario string
	delay    time.Duration
}

// timeToFirstRoot measures one replay. It reports false for a quiet
// scenario and when no message ever stated the expected root.
func timeToFirstRoot(
	e expectation, log replay.Log, result replay.Result,
) (firstRoot, bool) {
	if e.Quiet {
		return firstRoot{}, false
	}
	for _, a := range announcements(result) {
		if slices.Contains(e.Unrelated, a.root()) {
			continue
		}
		stated := reported{Root: a.root(), Cause: a.cause()}
		if !rootMatches(e.Root, stated) {
			continue
		}
		start := firstObservation(log, a.incident,
			failureStart(a.incident, log.Start))
		return firstRoot{scenario: e.Name, delay: a.at.Sub(start)}, true
	}
	return firstRoot{}, false
}

// firstRootLine summarises the time to the first correct root, naming
// the slowest scenarios and the ones that never stated it.
func firstRootLine(measured []firstRoot, never []string) string {
	if len(measured) == 0 {
		return "Time to first correct root: no cases.\n"
	}
	sorted := slices.Clone(measured)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].delay > sorted[j].delay
	})
	delays := make([]time.Duration, len(sorted))
	for i, m := range sorted {
		delays[len(sorted)-1-i] = m.delay
	}
	p95 := delays[(len(delays)*95+99)/100-1]
	parts := make([]string, 0, 5)
	for _, m := range sorted[:min(5, len(sorted))] {
		parts = append(parts, fmt.Sprintf("%s %s", m.scenario, m.delay))
	}
	line := fmt.Sprintf("Time to first correct root: p95 %s, max %s over "+
		"%d scenarios; slowest: %s.", p95, sorted[0].delay, len(sorted),
		strings.Join(parts, ", "))
	if len(never) > 0 {
		line += fmt.Sprintf(" Never stated: %s.", strings.Join(never, ", "))
	}
	return line + "\n"
}
