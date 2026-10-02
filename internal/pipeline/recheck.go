package pipeline

import (
	"container/heap"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// rechecks is a min-heap of entity re-evaluations. Scheduling an entity
// that is already pending keeps the earlier time.
type rechecks struct {
	items []recheckItem
	index map[inventory.EntityID]int
}

type recheckItem struct {
	id inventory.EntityID
	at time.Time
}

func newRechecks() *rechecks {
	return &rechecks{index: make(map[inventory.EntityID]int)}
}

func (r *rechecks) schedule(id inventory.EntityID, at time.Time) {
	if i, ok := r.index[id]; ok {
		if at.Before(r.items[i].at) {
			r.items[i].at = at
			heap.Fix(r, i)
		}
		return
	}
	heap.Push(r, recheckItem{id: id, at: at})
}

// due pops every entity whose recheck time has passed.
func (r *rechecks) due(now time.Time) []inventory.EntityID {
	var out []inventory.EntityID
	for len(r.items) > 0 && !r.items[0].at.After(now) {
		out = append(out, heap.Pop(r).(recheckItem).id)
	}
	return out
}

// next is the earliest pending time, or zero.
func (r *rechecks) next() time.Time {
	if len(r.items) == 0 {
		return time.Time{}
	}
	return r.items[0].at
}

func (r *rechecks) Len() int { return len(r.items) }

func (r *rechecks) Less(i, j int) bool {
	return r.items[i].at.Before(r.items[j].at)
}

func (r *rechecks) Swap(i, j int) {
	r.items[i], r.items[j] = r.items[j], r.items[i]
	r.index[r.items[i].id] = i
	r.index[r.items[j].id] = j
}

func (r *rechecks) Push(x any) {
	item := x.(recheckItem)
	r.index[item.id] = len(r.items)
	r.items = append(r.items, item)
}

func (r *rechecks) Pop() any {
	last := len(r.items) - 1
	item := r.items[last]
	r.items = r.items[:last]
	delete(r.index, item.id)
	return item
}
