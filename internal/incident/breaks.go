package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// BreakSnapshot is the incident's failing containers at one moment:
// when it first broke, or when it last got worse. Live container rows
// show the pods as they are now; a snapshot keeps them as they were
// when it mattered, after the pods were replaced.
type BreakSnapshot struct {
	At         time.Time
	Reason     string
	Containers []ContainerState
}

// Break reasons. Values are stable: they are persisted and served.
const (
	BreakFirst = "first break"
	BreakWorse = "got worse"
)

// Break bounds: the first break and the worst one, ten containers each.
const (
	maxBreaks          = 2
	maxBreakContainers = 10
)

// takeBreak stores a snapshot of p's failing containers, when it has
// any. The first one stays; a later one replaces the second, so the
// incident keeps how it began and how bad it got.
func (m *Manager) takeBreak(p *Incident, now time.Time, why string) {
	if m.model == nil {
		return
	}
	rows := failingContainers(readContainers(m.model, p.Root,
		failingMembers(p)))
	if len(rows) == 0 {
		return
	}
	if len(p.Journal.Breaks) == 0 {
		why = BreakFirst
	}
	snap := BreakSnapshot{At: now, Reason: why, Containers: rows}
	breaks := append([]BreakSnapshot(nil), p.Journal.Breaks...)
	if len(breaks) >= maxBreaks {
		breaks = breaks[:maxBreaks-1]
	}
	breaks = append(breaks, snap)
	p.Journal.Breaks = breaks
}

// failingContainers keeps the containers that fail: not ready,
// restarted with readiness unknown, or waiting or ended for a reason; at most
// maxBreakContainers, most restarted first.
func failingContainers(rows []ContainerState) []ContainerState {
	var out []ContainerState
	for _, row := range rows {
		if len(out) == maxBreakContainers {
			break
		}
		notReady := row.Ready != nil && !*row.Ready
		// A restart alone does not fail a container that is Ready with
		// nothing waiting or ended; it counts only when readiness is
		// unknown.
		unknown := row.Ready == nil && row.Restarts > 0
		if notReady || unknown || row.StateReason != "" ||
			row.LastReason != "" {
			out = append(out, row)
		}
	}
	return out
}

// failingMembers are the objects of p's failure findings, by name.
func failingMembers(p *Incident) []inventory.EntityID {
	var out []inventory.EntityID
	seen := map[inventory.EntityID]bool{}
	for _, f := range p.Members {
		if !f.Advisory && !seen[f.Entity] {
			seen[f.Entity] = true
			out = append(out, f.Entity)
		}
	}
	slices.SortFunc(out, compareIDs)
	return out
}
