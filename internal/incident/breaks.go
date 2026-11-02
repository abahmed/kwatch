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
		failingMembers(p)), breakCutoff(p, now))
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

// breakCutoff is when a last termination stops counting as part of
// p's break: the earlier of its onset and RecentWindow before now.
func breakCutoff(p *Incident, now time.Time) time.Time {
	cutoff := now.Add(-RecentWindow)
	if !p.Opened.IsZero() && p.Opened.Before(cutoff) {
		cutoff = p.Opened
	}
	return cutoff
}

// failingContainers keeps the containers that fail: not ready,
// restarted with readiness unknown, waiting or ended for a reason now,
// or ended for a reason at or after cutoff; at most maxBreakContainers,
// most restarted first.
func failingContainers(
	rows []ContainerState, cutoff time.Time,
) []ContainerState {
	var out []ContainerState
	for _, row := range rows {
		if len(out) == maxBreakContainers {
			break
		}
		if containerFailing(row, cutoff) {
			out = append(out, row)
		}
	}
	return out
}

// containerFailing reports whether row fails. A restart alone does not
// fail a container that is Ready with nothing waiting or ended; it
// counts only when readiness is unknown. A Ready container keeps the
// reason of its last termination after it recovers, so that reason
// counts only when the termination ended at or after cutoff.
func containerFailing(row ContainerState, cutoff time.Time) bool {
	if row.Ready != nil && !*row.Ready {
		return true
	}
	if (row.Ready == nil && row.Restarts > 0) || row.StateReason != "" {
		return true
	}
	if row.LastReason == "" {
		return false
	}
	if row.Ready == nil {
		return true
	}
	return !row.LastFinished.IsZero() && !row.LastFinished.Before(cutoff)
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
