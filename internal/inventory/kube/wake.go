package kube

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A cluster that is scaled down at night and scaled up in the morning
// does so all at once: many workloads go from zero replicas to some and
// many nodes join. For a while pods are pending, pulling images and
// failing their probes while the things they depend on come up. That is
// a wake-up. These functions find one from the model alone; nothing
// here knows which tool did the scaling.
const (
	// WakeMinWorkloads is how many workloads must start from zero
	// replicas together for the cluster to count as waking up.
	WakeMinWorkloads = 5
	// WakeMinNodes is how many nodes joining together count as one.
	WakeMinNodes = 3
	// WakeQuiet is how long after the last start the wake-up goes on;
	// starts closer together than this belong to the same wake-up.
	WakeQuiet = 10 * time.Minute
	// WakeMax caps a wake-up measured from its first start, so a
	// cluster that keeps scaling cannot hold failures forever.
	WakeMax = 30 * time.Minute
	// wakeLookback is how far back starts are searched. It leaves time
	// to summarise a wake-up after it ended.
	wakeLookback = 3 * time.Hour
	// wakeSlack lets a pod count as part of the wake-up when it was
	// created just before the start was recorded.
	wakeSlack = time.Minute
	// wakeRecompute is how long a scan of the cluster is reused: it
	// is asked once per pod evaluated.
	wakeRecompute = 10 * time.Second
)

// Wake is one wake-up of the cluster.
type Wake struct {
	// Start and Last are the first and the latest start of the run.
	Start, Last time.Time
	// Workloads are the workloads that started from zero replicas, in
	// name order.
	Workloads []inventory.EntityID
	// Nodes counts the nodes that joined.
	Nodes int
}

// End is when the wake-up is over: a quiet spell after the latest
// start, or WakeMax after the first, whichever comes first.
func (w Wake) End() time.Time {
	return minTime(w.Last.Add(WakeQuiet), w.Start.Add(WakeMax))
}

// Remaining is how much longer the wake-up lasts, or zero.
func (w Wake) Remaining(now time.Time) time.Duration {
	return max(0, w.End().Sub(now))
}

// Covers reports whether something created at created, such as a pod,
// belongs to the wake-up rather than to the cluster that was already up.
func (w Wake) Covers(created time.Time) bool {
	return !created.IsZero() && !created.Before(w.Start.Add(-wakeSlack))
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// start is one workload or node starting.
type start struct {
	at       time.Time
	workload inventory.EntityID
}

// LatestWake is the most recent wake-up in the model, over or still on.
// It reports false when the cluster did not wake up lately.
func LatestWake(r inventory.Reader, now time.Time) (Wake, bool) {
	return wakeMemo.get(r, now, scanWake)
}

// ActiveWake is the wake-up going on at now, if any.
func ActiveWake(r inventory.Reader, now time.Time) (Wake, bool) {
	w, ok := LatestWake(r, now)
	return w, ok && w.Remaining(now) > 0
}

// PodWakeRemaining is how much longer the wake-up that created the pod
// lasts, or zero when the pod is older or no wake-up is on.
func PodWakeRemaining(
	r inventory.Reader, pod inventory.Entity, now time.Time,
) time.Duration {
	created, ok := pod.Attribute(AttrCreated)
	if !ok {
		return 0
	}
	w, on := ActiveWake(r, now)
	if !on || !w.Covers(created.Value.AsTime()) {
		return 0
	}
	return w.Remaining(now)
}

func scanWake(r inventory.Reader, now time.Time) (Wake, bool) {
	since := now.Add(-wakeLookback)
	var starts []start
	for _, kind := range []inventory.Kind{KindDeployment, KindStatefulSet} {
		for _, id := range r.Entities(kind) {
			if at, ok := startedFromZero(r, id, since); ok {
				starts = append(starts, start{at, id})
			}
		}
	}
	for _, id := range r.Entities(KindNode) {
		if node, ok := r.Entity(id); ok {
			if a, ok := node.Attribute(AttrCreated); ok &&
				a.Value.AsTime().After(since) {
				starts = append(starts, start{at: a.Value.AsTime()})
			}
		}
	}
	slices.SortFunc(starts, func(a, b start) int {
		return a.at.Compare(b.at)
	})
	return latestRun(starts)
}

// latestRun is the newest run of starts, each less than WakeQuiet after
// the one before, that is big enough to be a wake-up.
func latestRun(starts []start) (Wake, bool) {
	for end := len(starts); end > 0; {
		first := end - 1
		for first > 0 &&
			starts[first].at.Sub(starts[first-1].at) < WakeQuiet {
			first--
		}
		run := Wake{Start: starts[first].at, Last: starts[end-1].at}
		for _, s := range starts[first:end] {
			if s.workload.IsZero() {
				run.Nodes++
			} else {
				run.Workloads = append(run.Workloads, s.workload)
			}
		}
		if len(run.Workloads) >= WakeMinWorkloads ||
			run.Nodes >= WakeMinNodes {
			slices.SortFunc(run.Workloads, func(a, b inventory.EntityID) int {
				return strings.Compare(a.String(), b.String())
			})
			return run, true
		}
		end = first
	}
	return Wake{}, false
}

// startedFromZero is when the workload's replicas last went from zero to
// some within the search, per its recorded changes.
func startedFromZero(
	r inventory.Reader, id inventory.EntityID, since time.Time,
) (time.Time, bool) {
	var at time.Time
	for _, change := range r.Changes(id, since) {
		for _, f := range change.Fields {
			after, err := strconv.Atoi(f.After)
			if f.Path == "spec.replicas" && f.Before == "0" &&
				err == nil && after > 0 && change.At.After(at) {
				at = change.At
			}
		}
	}
	return at, !at.IsZero()
}
