package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Planned operations. Some clusters are put to sleep at the same time
// every evening (many workloads scaled to zero, nodes switched off and
// removed) and woken at the same time every morning. kwatch keeps a
// short, bounded history of the mass scale-downs and scale-ups it saw.
// One that happens within PlannedTolerance of the time of day of the
// same kind of operation on at least PlannedMinDays of the previous
// PlannedDays days is a scheduled operation: what it explains is held
// without a message (planned_hold.go), and the digest carries one line
// for it. Nothing is predicted: only operations seen are compared, and
// the first ones, with no history, are treated as before.
const (
	// PlannedDays is how many previous days are compared.
	PlannedDays = 3
	// PlannedMinDays is how many of them must have had the same kind
	// of operation at about the same time.
	PlannedMinDays = 2
	// PlannedTolerance is how far apart in time of day two operations
	// may start and still count as the same schedule.
	PlannedTolerance = 30 * time.Minute
	// PlannedGrace is how long after the scale-up that follows a
	// scheduled scale-down is over what the scale-down explained may
	// still fail: past it, what did not come back is told as usual.
	PlannedGrace = kube.LeftBehindGrace
	// plannedMaxHold caps the hold of a scheduled scale-down when no
	// scale-up follows it.
	plannedMaxHold = 24 * time.Hour
	// plannedKeep is how long an operation is remembered.
	plannedKeep = (PlannedDays + 2) * 24 * time.Hour
	// plannedMaxOps bounds the history; plannedMaxNames bounds the
	// workloads one operation remembers.
	plannedMaxOps   = 32
	plannedMaxNames = 256
)

// PlannedOp is one mass scale-down or scale-up kwatch saw. It is
// persisted with the digest state, so the history outlives a restart.
type PlannedOp struct {
	// Down marks a scale-down; false is a scale-up.
	Down bool `json:",omitempty"`
	// At and Last are its first and latest change.
	At   time.Time
	Last time.Time `json:",omitempty"`
	// Workloads are the workloads it scaled, by EntityID.String(), at
	// most plannedMaxNames of them; Scaled counts them all.
	Workloads []string `json:",omitempty"`
	Scaled    int      `json:",omitempty"`
	// Nodes counts the nodes it removed or added.
	Nodes int `json:",omitempty"`
}

// samePlannedOp reports two sightings of one operation: same kind, and
// their changes overlap within kube.ScaleDownSpan.
func samePlannedOp(a, b PlannedOp) bool {
	return a.Down == b.Down &&
		!a.At.After(lastOf(b).Add(kube.ScaleDownSpan)) &&
		!b.At.After(lastOf(a).Add(kube.ScaleDownSpan))
}

func lastOf(op PlannedOp) time.Time {
	if op.Last.Before(op.At) {
		return op.At
	}
	return op.Last
}

// merge folds a newer sighting of the same operation into op.
func (op PlannedOp) merge(seen PlannedOp) PlannedOp {
	op.At = minTime(op.At, seen.At)
	if lastOf(seen).After(lastOf(op)) {
		op.Last = lastOf(seen)
	}
	for _, name := range seen.Workloads {
		if !slices.Contains(op.Workloads, name) &&
			len(op.Workloads) < plannedMaxNames {
			op.Workloads = append(op.Workloads, name)
		}
	}
	op.Scaled = max(op.Scaled, seen.Scaled, len(op.Workloads))
	op.Nodes = max(op.Nodes, seen.Nodes)
	return op
}

// ClonePlanned copies ops deeply, for a writer on another goroutine.
func ClonePlanned(ops []PlannedOp) []PlannedOp {
	out := slices.Clone(ops)
	for i := range out {
		out[i].Workloads = slices.Clone(out[i].Workloads)
	}
	return out
}

// EqualPlanned reports two histories that hold the same operations.
func EqualPlanned(a, b []PlannedOp) bool {
	return slices.EqualFunc(a, b, func(x, y PlannedOp) bool {
		return x.Down == y.Down && x.At.Equal(y.At) &&
			x.Last.Equal(y.Last) && x.Scaled == y.Scaled &&
			x.Nodes == y.Nodes && slices.Equal(x.Workloads, y.Workloads)
	})
}

// plannedHistory is the operations seen, oldest first.
type plannedHistory []PlannedOp

// record adds a sighting, merged into the operation it belongs to, and
// forgets what is older than plannedKeep before now.
func (h plannedHistory) record(seen PlannedOp, now time.Time) plannedHistory {
	merged := false
	for i := range h {
		if samePlannedOp(h[i], seen) {
			h[i], merged = h[i].merge(seen), true
		}
	}
	if !merged {
		h = append(h, PlannedOp{Down: seen.Down, At: seen.At}.merge(seen))
	}
	slices.SortStableFunc(h, func(a, b PlannedOp) int {
		return a.At.Compare(b.At)
	})
	return h.prune(now)
}

// prune drops operations older than plannedKeep and the oldest past
// plannedMaxOps.
func (h plannedHistory) prune(now time.Time) plannedHistory {
	kept := h[:0]
	for _, op := range h {
		if now.Sub(lastOf(op)) < plannedKeep {
			kept = append(kept, op)
		}
	}
	if len(kept) > plannedMaxOps {
		kept = kept[len(kept)-plannedMaxOps:]
	}
	return kept
}

// recurs counts the previous PlannedDays days on which an operation of
// the same kind began within PlannedTolerance of op's time of day.
func (h plannedHistory) recurs(op PlannedOp) int {
	seen := 0
	for day := 1; day <= PlannedDays; day++ {
		want := op.At.Add(-time.Duration(day) * 24 * time.Hour)
		for _, past := range h {
			gap := past.At.Sub(want)
			if past.Down == op.Down && gap <= PlannedTolerance &&
				gap >= -PlannedTolerance {
				seen++
				break
			}
		}
	}
	return seen
}

// scheduled reports an operation that recurs on enough earlier days.
func (h plannedHistory) scheduled(op PlannedOp) (int, bool) {
	seen := h.recurs(op)
	return seen, seen >= PlannedMinDays
}

// upAfter is the first scale-up that began after the scale-down down.
func (h plannedHistory) upAfter(down PlannedOp) (PlannedOp, bool) {
	for _, op := range h {
		if !op.Down && op.At.After(down.At) {
			return op, true
		}
	}
	return PlannedOp{}, false
}

// effectEnd is when what the scheduled scale-down down explains stops
// being explained: PlannedGrace after the scale-up that followed it was
// over, or plannedMaxHold after it began while none followed.
func (h plannedHistory) effectEnd(down PlannedOp) time.Time {
	up, ok := h.upAfter(down)
	if !ok {
		return down.At.Add(plannedMaxHold)
	}
	return wakeEnd(up).Add(PlannedGrace)
}

// wakeEnd is when the scale-up op was over, as kube.Wake words it.
func wakeEnd(op PlannedOp) time.Time {
	return kube.Wake{Start: op.At, Last: lastOf(op)}.End()
}

// ScheduledOp is a scheduled operation that is over, for the one
// digest line that tells it: Seen of the last PlannedDays days had the
// same kind of operation at about the same time.
type ScheduledOp struct {
	PlannedOp
	Seen int
}

// LatestScheduled is the newest operation of ops that is over at now
// and recurs on enough earlier days. A scale-down is over
// kube.ScaleDownSpan after its latest change; a scale-up when its
// wake-up is.
func LatestScheduled(ops []PlannedOp, now time.Time) (ScheduledOp, bool) {
	h := plannedHistory(ops)
	for i := len(h) - 1; i >= 0; i-- {
		op := h[i]
		over := wakeEnd(op)
		if op.Down {
			over = lastOf(op).Add(kube.ScaleDownSpan)
		}
		if now.Before(over) {
			continue
		}
		if seen, ok := h.scheduled(op); ok {
			return ScheduledOp{PlannedOp: op, Seen: seen}, true
		}
	}
	return ScheduledOp{}, false
}

// ScheduledUp reports a scale-up of ops begun at start that recurs on
// enough earlier days: its digest line replaces the wake-up's.
func ScheduledUp(ops []PlannedOp, start time.Time) bool {
	h := plannedHistory(ops)
	for _, op := range h {
		if samePlannedOp(op, PlannedOp{At: start}) {
			_, ok := h.scheduled(op)
			return ok
		}
	}
	return false
}
