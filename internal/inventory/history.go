package inventory

import (
	"slices"
	"sort"
	"time"
)

// DefaultMaxRecentChanges bounds the cluster-wide change history that
// change sets are built from.
const DefaultMaxRecentChanges = 1024

// DefaultMaxChurnChanges bounds the separate history of pod and
// ReplicaSet creations and deletions. A node drain or a large rollout
// creates hundreds of them within seconds; keeping them apart means they
// cannot push a Deployment or ConfigMap edit out of the cluster history.
const DefaultMaxChurnChanges = 256

// DefaultMaxScaleChanges bounds the separate history of replica-count
// edits. An autoscaler can change a workload's replicas every minute;
// kept apart, those edits cannot push a rollout or a config edit out of
// the cluster history.
const DefaultMaxScaleChanges = 256

// DefaultMaxHealthMarks bounds the health marks kept per entity.
const DefaultMaxHealthMarks = 16

// HealthMark records that an entity started or stopped failing, as the
// detectors judged it. The pipeline feeds marks back into the model so
// change outcomes can see what happened downstream of a change.
type HealthMark struct {
	At      time.Time
	Failing bool
	// Mode is the failure mode that started or stopped, such as
	// "CrashLoop".
	Mode string
	// Revision is the workload revision a healthy mark was taken at,
	// when known. Failing marks leave it empty.
	Revision string `json:",omitempty"`
}

// history is the model's cluster-wide memory of recent changes. The
// model's lock guards the rings and counters.
type history struct {
	// recent holds meaningful changes; churn holds pod and ReplicaSet
	// creations and deletions; scale holds replica-count-only edits.
	// All are oldest first.
	recent []changeEntry
	churn  []changeEntry
	scale  []changeEntry
	// pinned holds the older changes a restart reloaded for open
	// incidents (history_pinned.go), oldest first.
	pinned []changeEntry
	// seq numbers appended changes, so the change-set cache can fold in
	// only the changes it has not seen.
	seq uint64
	// evictions counts changes that left the rings, so the cache knows
	// when to drop members.
	evictions uint64
	cache     *setCache
	baselines *Baselines
}

// changeEntry is one change with its history sequence number.
type changeEntry struct {
	seq    uint64
	change Change
}

func newHistory() history {
	return history{cache: newSetCache(), baselines: NewBaselines()}
}

func (h *history) addChange(change Change) {
	h.seq++
	entry := changeEntry{seq: h.seq, change: cloneChange(change)}
	switch {
	case isChurn(change):
		h.churn = h.appendBounded(h.churn, entry, DefaultMaxChurnChanges)
	case ScaleOnly(change):
		h.scale = h.appendBounded(h.scale, entry, DefaultMaxScaleChanges)
	default:
		h.recent = h.appendBounded(h.recent, entry,
			DefaultMaxRecentChanges)
	}
}

// appendBounded adds entry and evicts the oldest entries beyond limit. It
// reslices instead of copying, so a full ring costs the same per change as
// an empty one. The evicted slots stay unused at the front of the backing
// array until append runs out of room and moves the live entries to a new
// array, so the array never holds more than 2*limit slots.
func (h *history) appendBounded(
	ring []changeEntry, entry changeEntry, limit int,
) []changeEntry {
	ring = append(ring, entry)
	if over := len(ring) - limit; over > 0 {
		clear(ring[:over]) // release the evicted changes now
		ring = ring[over:]
		h.evictions += uint64(over)
	}
	return ring
}

// isChurn reports a pod or ReplicaSet being created or deleted:
// controller bookkeeping that a drain or rollout produces in bulk.
func isChurn(change Change) bool {
	kind := change.Entity.Kind
	return (kind == "pod" || kind == "replicaset") &&
		(change.Created || change.Deleted)
}

// ScaleOnly reports an edit of nothing but the replica count, which an
// autoscaler makes in bulk. Such edits are kept apart from the other
// changes, in the cluster history and in each entity's, so they never
// push a rollout or a config edit out.
func ScaleOnly(change Change) bool {
	if change.Created || change.Deleted || len(change.Fields) == 0 {
		return false
	}
	for _, f := range change.Fields {
		if f.Path != scalePath {
			return false
		}
	}
	return true
}

// scalePath is the field of a replica-count edit.
const scalePath = "spec.replicas"

// boundChanges keeps at most limit of an entity's changes of each class,
// scale-only edits and the rest, evicting the oldest of the class the
// latest change belongs to.
func boundChanges(changes []Change, limit int) []Change {
	scale := ScaleOnly(changes[len(changes)-1])
	n, first := 0, -1
	for i := range changes {
		if ScaleOnly(changes[i]) == scale {
			if first < 0 {
				first = i
			}
			n++
		}
	}
	if n <= limit {
		return changes
	}
	return slices.Delete(changes, first, first+1)
}

func (h *history) prune(before time.Time) {
	h.recent = h.pruneRing(h.recent, before)
	h.churn = h.pruneRing(h.churn, before)
	h.scale = h.pruneRing(h.scale, before)
	h.pinned = h.pruneRing(h.pinned, pinnedFrom(before))
}

func (h *history) pruneRing(
	ring []changeEntry, before time.Time,
) []changeEntry {
	kept := ring[:0]
	for _, entry := range ring {
		if !entry.change.At.Before(before) {
			kept = append(kept, entry)
		}
	}
	h.evictions += uint64(len(ring) - len(kept))
	clear(ring[len(kept):]) // release the pruned changes
	return kept
}

// rings returns every change ring.
func (h *history) rings() [][]changeEntry {
	return [][]changeEntry{h.recent, h.churn, h.scale, h.pinned}
}

// entriesAfter returns the live changes numbered above seq, in sequence
// order.
func (h *history) entriesAfter(seq uint64) []changeEntry {
	var out []changeEntry
	for _, ring := range h.rings() {
		for _, entry := range ring {
			if entry.seq > seq {
				out = append(out, entry)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].seq < out[j].seq })
	return out
}

// liveSeqs returns the sequence numbers still in the rings.
func (h *history) liveSeqs() map[uint64]bool {
	live := make(map[uint64]bool,
		len(h.recent)+len(h.churn)+len(h.scale)+len(h.pinned))
	for _, ring := range h.rings() {
		for _, entry := range ring {
			live[entry.seq] = true
		}
	}
	return live
}

// MarkHealth records a health transition of id at at.
func (m *Model) MarkHealth(
	id EntityID, failing bool, mode string, at time.Time,
) {
	if id.IsZero() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.recordFor(id)
	rec.health = append(rec.health, HealthMark{
		At: at, Failing: failing, Mode: mode,
	})
	if over := len(rec.health) - DefaultMaxHealthMarks; over > 0 {
		rec.health = append(rec.health[:0:0], rec.health[over:]...)
	}
}

// HealthMarks returns id's health marks at or after since, oldest first.
func (m *Model) HealthMarks(id EntityID, since time.Time) []HealthMark {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthMarksLocked(id, since)
}

func (m *Model) healthMarksLocked(
	id EntityID, since time.Time,
) []HealthMark {
	rec := m.records[id]
	if rec == nil {
		return nil
	}
	var out []HealthMark
	for _, mark := range rec.health {
		if !mark.At.Before(since) {
			out = append(out, mark)
		}
	}
	return out
}

// Baselines returns the model's workload baselines. They are safe for
// concurrent use and live as long as the model.
func (m *Model) Baselines() *Baselines {
	return m.history.baselines
}

func pruneHealth(marks []HealthMark, before time.Time) []HealthMark {
	kept := marks[:0]
	for _, mark := range marks {
		if !mark.At.Before(before) {
			kept = append(kept, mark)
		}
	}
	return kept
}
