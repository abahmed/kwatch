package correlation

import (
	"sort"
	"time"
)

// flushGroupBuffers emits or updates a smart-group incident per buffer whose
// grouping window has elapsed. Caller must hold e.mu.
func (e *Engine) flushGroupBuffers(now time.Time) []transition {
	var pending []transition

	ready := make([]string, 0, len(e.groupBuffers))
	for gk, pg := range e.groupBuffers {
		if len(pg.entries) == 0 ||
			!now.After(pg.firstSeen.Add(e.config.SmartGroupingWindow)) {
			continue
		}
		ready = append(ready, gk)
	}
	// Map iteration is random and merging depends on which buffers are seen
	// together; sort so a given set of buffers always produces the same result.
	sort.Strings(ready)

	merged := e.mergeNamespaceFanOut(ready, now)
	pending = append(pending, merged.transitions...)
	e.pruneFanOutWindows(now)

	for _, gk := range ready {
		if merged.consumed[gk] {
			delete(e.groupBuffers, gk)
			continue
		}
		if t, ok := e.flushOneGroup(gk, e.groupBuffers[gk], now); ok {
			pending = append(pending, t)
		}
		delete(e.groupBuffers, gk)
	}
	return pending
}
