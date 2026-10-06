package coverage

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The timings of the coverage check. What the check is for, and how
// the pieces fit, is in doc.go.
const (
	// Every is how often the coverage check runs.
	Every = 5 * time.Minute
	// After is how long a workload must have been below its desired
	// replicas; it is longer than kube.BootWindow, so a node pool that
	// is still booting is never mistaken for a lost incident.
	After = 15 * time.Minute
	// Retry is how long a workload that was just handed back is left
	// alone, so a workload the manager cannot cover is not logged every
	// five minutes.
	Retry = 30 * time.Minute
)

// Watch is the state of the check across runs. The zero value is ready.
type Watch struct {
	checked time.Time
	// shortSince is when each workload was first seen below desired.
	shortSince map[inventory.EntityID]time.Time
	// restarts is each short workload's restart total at the last run.
	restarts map[inventory.EntityID]float64
	// handed is when each workload was last handed back.
	handed map[inventory.EntityID]time.Time
}

// FailingWorkloads returns the workloads that have been below their
// desired replicas for After and either have none ready or keep
// restarting. It remembers what it saw for the next run.
func (w *Watch) FailingWorkloads(
	model inventory.Reader, now time.Time,
) []inventory.EntityID {
	if w.shortSince == nil {
		w.shortSince = map[inventory.EntityID]time.Time{}
		w.restarts = map[inventory.EntityID]float64{}
		w.handed = map[inventory.EntityID]time.Time{}
	}
	var out []inventory.EntityID
	seen := map[inventory.EntityID]bool{}
	for _, kind := range kube.WorkloadKinds {
		for _, id := range model.Entities(kind) {
			seen[id] = true
			if w.failing(model, id, now) {
				out = append(out, id)
			}
		}
	}
	for id := range w.shortSince {
		if !seen[id] {
			w.forget(id)
		}
	}
	for id := range w.handed {
		if !seen[id] {
			delete(w.handed, id)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

// failing judges one workload and updates what is remembered of it.
func (w *Watch) failing(
	model inventory.Reader, id inventory.EntityID, now time.Time,
) bool {
	desired, ready, ok := incident.ReplicasOf(model, id)
	if !ok || desired == 0 || ready >= desired {
		w.forget(id)
		return false
	}
	since, short := w.shortSince[id]
	if !short {
		since = now
		w.shortSince[id] = now
	}
	r, _ := incident.ReadinessOf(model, id)
	before, known := w.restarts[id]
	w.restarts[id] = r.Restarts
	if now.Sub(since) < After {
		return false
	}
	return ready == 0 || (known && r.Restarts > before)
}

func (w *Watch) forget(id inventory.EntityID) {
	delete(w.shortSince, id)
	delete(w.restarts, id)
}

// Due reports that the check should run now, and starts the next wait.
func (w *Watch) Due(now time.Time) bool {
	if !w.checked.IsZero() && now.Sub(w.checked) < Every {
		return false
	}
	w.checked = now
	return true
}

// Settled reports a workload that may be handed back now: it was not
// handed back within Retry.
func (w *Watch) Settled(id inventory.EntityID, now time.Time) bool {
	return now.Sub(w.handed[id]) >= Retry
}

// HandBack records that id was handed back at now.
func (w *Watch) HandBack(id inventory.EntityID, now time.Time) {
	w.handed[id] = now
}
