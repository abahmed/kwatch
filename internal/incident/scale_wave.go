package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Scaling waves. Many clusters add and remove nodes on a schedule: a
// nightly shutdown, a morning wake, a batch window. While nodes come and
// go in numbers, pods wait for a node, pull their images on a cold node,
// and lose the node they ran on. Those are the scaling at work, not an
// outage, and they clear once the new nodes are up. A page for them
// wakes someone for nothing.
//
// So kwatch watches the node set. Enough nodes added or removed within
// scaleWindow is a wave. An incident opened during it whose failures
// are all typical scaling side effects (scale_hold.go) keeps settling,
// without a message, until the wave is over (no node added or removed
// for scaleQuiet) and scaleGrace more has passed, never longer than
// scaleMaxHold from its onset, and scaleMaxHoldServing when users
// lose traffic or nothing serves: a failure that outlives the wave is
// told as usual, and pages when it calls for a page; one that cleared
// closes silently. A crash loop, an OOM kill, a failed image pull or a
// bad release is never held, and neither is a failure the wave's
// nodes have nothing to do with.

const (
	// scaleWindow is the stretch the node changes of a wave fall in.
	scaleWindow = 10 * time.Minute
	// scaleMinNodes is the fewest nodes added or removed within
	// scaleWindow that make a wave, whatever the cluster's size: one or
	// two nodes replaced is ordinary churn (node_churn.go).
	scaleMinNodes = 3
	// scaleShare is the share of the cluster's nodes, in percent, that
	// must change in a larger cluster, where a few nodes added by the
	// autoscaler are routine.
	scaleShare = 20
	// scaleQuiet is how long no node may be added or removed before a
	// wave is over.
	scaleQuiet = 5 * time.Minute
	// scaleGrace is how long after the wave the new nodes get to pull
	// images and start the pods that waited for them.
	scaleGrace = 5 * time.Minute
	// scaleMaxHold caps the hold of one incident from its onset, so a
	// cluster that never stops scaling cannot hold a page for long.
	scaleMaxHold = 30 * time.Minute
	// scaleMaxHoldServing is the cap when users lose traffic or a
	// user-facing workload has no ready replica: a node that was
	// taken away and never replaced looks like scaling, too.
	scaleMaxHoldServing = 15 * time.Minute
)

// scaleWave remembers the nodes seen and the recent node additions and
// removals, and the wave they make, if any.
type scaleWave struct {
	nodes map[inventory.EntityID]bool
	// size is the larger of the node counts before and after the
	// latest look, for scaleShare: a cluster that shrank for good is
	// judged by the size it has now.
	size int
	// unsynced says the nodes are not all listed yet, so what is in
	// the model is no baseline (noteSync).
	unsynced bool
	// recent are the times of the node additions and removals within
	// scaleWindow of the latest look.
	recent []time.Time
	// started is when the current or latest wave began (its first
	// change), and last its latest change; zero when none was seen.
	started, last time.Time
}

// noteSync records whether the Node kind is fully listed, from the
// snapshot's Synced; nil is unknown and counts as listed. A baseline
// taken from half a list would read the rest of it as nodes joining.
func (w *scaleWave) noteSync(synced func(inventory.Kind) bool) {
	w.unsynced = synced != nil && !synced(kube.KindNode)
}

// observe compares the nodes of model with the last look. The first
// look with every node listed is the baseline: it is the state kwatch
// started in.
func (w *scaleWave) observe(model inventory.Reader, now time.Time) {
	if model == nil {
		return
	}
	ids := model.Entities(kube.KindNode)
	current := make(map[inventory.EntityID]bool, len(ids))
	for _, id := range ids {
		current[id] = true
	}
	if w.nodes == nil {
		if len(current) > 0 && !w.unsynced {
			w.nodes, w.size = current, len(current)
		}
		return
	}
	changes := 0
	for id := range current {
		if !w.nodes[id] {
			changes++
		}
	}
	for id := range w.nodes {
		if !current[id] {
			changes++
		}
	}
	w.size = max(len(w.nodes), len(current))
	w.nodes = current
	w.note(now, changes)
}

// note records changes node additions and removals at now.
func (w *scaleWave) note(now time.Time, changes int) {
	kept := w.recent[:0]
	for _, at := range w.recent {
		if now.Sub(at) < scaleWindow {
			kept = append(kept, at)
		}
	}
	w.recent = kept
	for range changes {
		w.recent = append(w.recent, now)
	}
	if changes == 0 {
		return
	}
	if w.on(now) {
		w.last = now
		return
	}
	if fresh := w.fresh(); len(fresh) >= w.threshold() {
		w.started, w.last = fresh[0], now
	}
}

// fresh are the recent changes after the latest wave's last change.
// Only they can begin a new wave: the changes of a wave that is over
// are spent, or one late node would start it again.
func (w *scaleWave) fresh() []time.Time {
	for i, at := range w.recent {
		if at.After(w.last) {
			return w.recent[i:]
		}
	}
	return nil
}

// threshold is how many node changes within scaleWindow make a wave.
func (w *scaleWave) threshold() int {
	return max(scaleMinNodes, (w.size*scaleShare+99)/100)
}

// on reports a wave that is not over at now: a node was added or
// removed within scaleQuiet.
func (w *scaleWave) on(now time.Time) bool {
	return !w.last.IsZero() && now.Before(w.last.Add(scaleQuiet))
}

// holdEnd is when the latest wave's hold ends: scaleGrace after the
// wave is over. Zero when there was no wave.
func (w *scaleWave) holdEnd() time.Time {
	if w.last.IsZero() {
		return time.Time{}
	}
	return w.last.Add(scaleQuiet + scaleGrace)
}

// ScalingState says whether pages are held because the cluster is
// scaling: since when nodes are added or removed in numbers, and when
// the hold is expected to end if no node changes again (the wave over,
// plus the grace).
type ScalingState struct {
	Held     bool
	Since    time.Time
	Expected time.Time
}

// Scaling is the cluster's scaling state at now.
func (m *Manager) Scaling(now time.Time) ScalingState {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.wave
	end := w.holdEnd()
	if end.IsZero() || !now.Before(end) {
		return ScalingState{}
	}
	return ScalingState{Held: true, Since: w.started, Expected: end}
}
