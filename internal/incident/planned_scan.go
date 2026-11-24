package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	// plannedScanEvery is how often the model is searched for mass
	// scale-downs and scale-ups.
	plannedScanEvery = 30 * time.Second
	// plannedLookback is how far back a search looks.
	plannedLookback = 3 * time.Hour
)

// plannedOps is what the manager knows of planned operations: the
// persisted history, and what the latest looks found.
type plannedOps struct {
	history plannedHistory
	scanned time.Time
	// nodes are the nodes of the last look, each with when it stopped
	// being ready (zero while ready).
	nodes map[inventory.EntityID]time.Time
	// gone are the nodes removed within plannedLookback, each with when
	// it stopped reporting or, when it never did, was removed.
	gone map[inventory.EntityID]time.Time
	// down are the workloads and nodes of the latest mass scale-down.
	down map[inventory.EntityID]bool
}

// downStart is one workload set to zero or one node lost or removed.
type downStart struct {
	at time.Time
	id inventory.EntityID
}

// notePlanned looks at the model for mass scale-downs and scale-ups at
// most every plannedScanEvery, and records what it finds. A node
// removed between two looks is dated at the look that misses it,
// unless it stopped being ready before.
func (m *Manager) notePlanned(model inventory.Reader, now time.Time) {
	o := &m.plan
	if model == nil || (!o.scanned.IsZero() && !now.Before(o.scanned) &&
		now.Sub(o.scanned) < plannedScanEvery) {
		return
	}
	o.scanned = now
	o.noteNodes(model, now)
	if down, ok := o.scanDown(model, now); ok {
		o.history = o.history.record(down, now)
	}
	if w, ok := kube.LatestWake(model, now); ok {
		o.history = o.history.record(upOf(w), now)
	}
}

// upOf is the scale-up a wake-up is.
func upOf(w kube.Wake) PlannedOp {
	op := PlannedOp{At: w.Start, Last: w.Last, Nodes: w.Nodes,
		Scaled: len(w.Workloads)}
	for _, id := range w.Workloads {
		if len(op.Workloads) < plannedMaxNames {
			op.Workloads = append(op.Workloads, id.String())
		}
	}
	return op
}

// noteNodes remembers when each node stopped being ready and when a
// node was removed: a removed node is gone from the model.
func (o *plannedOps) noteNodes(model inventory.Reader, now time.Time) {
	current := map[inventory.EntityID]time.Time{}
	for _, id := range model.Entities(kube.KindNode) {
		current[id] = notReadySince(model, id)
	}
	for id, lost := range o.nodes {
		if _, there := current[id]; there {
			continue
		}
		if o.gone == nil {
			o.gone = map[inventory.EntityID]time.Time{}
		}
		if lost.IsZero() {
			lost = now
		}
		o.gone[id] = lost
	}
	for id, at := range o.gone {
		if now.Sub(at) > plannedLookback {
			delete(o.gone, id)
		}
	}
	o.nodes = current
}

// notReadySince is when the node stopped being ready, as the cluster
// dates it; zero while it is ready or its readiness is not known.
func notReadySince(model inventory.Reader, id inventory.EntityID) time.Time {
	ready, ok := model.Attribute(id, kube.AttrReady)
	if !ok {
		return time.Time{}
	}
	if v, isBool := ready.Value.AsBool(); !isBool || v {
		return time.Time{}
	}
	if since, ok := model.Attribute(id, kube.AttrReadySince); ok &&
		!since.Value.AsTime().IsZero() {
		return since.Value.AsTime()
	}
	return ready.Since
}

// scanDown finds the newest mass scale-down within plannedLookback:
// workloads set to zero and nodes lost or removed, each less than
// kube.ScaleDownSpan after the one before, with at least
// kube.ScaleDownMin workloads or kube.WakeMinNodes nodes.
func (o *plannedOps) scanDown(
	model inventory.Reader, now time.Time,
) (PlannedOp, bool) {
	since := now.Add(-plannedLookback)
	var starts []downStart
	for _, kind := range []inventory.Kind{
		kube.KindDeployment, kube.KindStatefulSet,
	} {
		for _, id := range model.Entities(kind) {
			if at, ok := kube.ZeroedAt(model, id, since); ok {
				starts = append(starts, downStart{at, id})
			}
		}
	}
	for id, at := range o.nodes {
		if at.After(since) {
			starts = append(starts, downStart{at, id})
		}
	}
	for id, at := range o.gone {
		starts = append(starts, downStart{at, id})
	}
	slices.SortFunc(starts, func(a, b downStart) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}
		return compareIDs(a.id, b.id)
	})
	return o.latestDown(starts)
}

// latestDown is the newest run of starts big enough to be a mass
// scale-down; it also remembers the run's workloads and nodes.
func (o *plannedOps) latestDown(starts []downStart) (PlannedOp, bool) {
	for end := len(starts); end > 0; {
		first := end - 1
		for first > 0 && starts[first].at.Sub(starts[first-1].at) <
			kube.ScaleDownSpan {
			first--
		}
		op := PlannedOp{Down: true, At: starts[first].at,
			Last: starts[end-1].at}
		down := map[inventory.EntityID]bool{}
		for _, s := range starts[first:end] {
			down[s.id] = true
			if s.id.Kind == kube.KindNode {
				op.Nodes++
				continue
			}
			op.Scaled++
			if len(op.Workloads) < plannedMaxNames {
				op.Workloads = append(op.Workloads, s.id.String())
			}
		}
		if op.Scaled >= kube.ScaleDownMin || op.Nodes >= kube.WakeMinNodes {
			o.down = down
			return op, true
		}
		end = first
	}
	return PlannedOp{}, false
}
