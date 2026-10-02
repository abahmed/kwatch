package detection

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// onsets remembers, per entity, when each ongoing bad state began. A
// detector's waiting period must run from the start of the bad state:
// the attribute it reads changes with every new value (one more replica
// lost, a few milliseconds more latency), so the attribute's own Since
// would restart the wait on every change.
//
// A state is kept only while a detector keeps asking for it: after each
// evaluation of an entity, the states nobody asked for are forgotten.
// An entity that is gone is evaluated without detectors, so its states
// are forgotten too.
type onsets struct {
	mu       sync.Mutex
	byEntity map[inventory.EntityID]map[string]time.Time
}

func newOnsets() *onsets {
	return &onsets{byEntity: map[inventory.EntityID]map[string]time.Time{}}
}

// begin starts one evaluation of id: it returns the states known so far.
func (o *onsets) begin(id inventory.EntityID) *onsetScope {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &onsetScope{known: o.byEntity[id], kept: map[string]time.Time{}}
}

// finish keeps only the states the evaluation asked for.
func (o *onsets) finish(id inventory.EntityID, scope *onsetScope) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(scope.kept) == 0 {
		delete(o.byEntity, id)
		return
	}
	o.byEntity[id] = scope.kept
}

// onsetScope is the onset state of one evaluation of one entity.
type onsetScope struct {
	known map[string]time.Time
	kept  map[string]time.Time
}

// Onset returns when the bad state named key of the evaluated entity
// began. Call it only while the state holds. The first call for a state
// returns firstSeen (or now when firstSeen is zero); later evaluations
// return that same time until an evaluation no longer asks for the
// state. Without a registry, as in detector unit tests, it returns
// firstSeen.
func (c Context) Onset(key string, firstSeen time.Time) time.Time {
	if firstSeen.IsZero() {
		firstSeen = c.Now
	}
	if c.onsets == nil {
		return firstSeen
	}
	at, ok := c.onsets.known[key]
	if !ok {
		at = firstSeen
	}
	c.onsets.kept[key] = at
	return at
}

// Ongoing reports whether the previous evaluation of the entity asked
// for the state named key, that is, whether the state held then. It does
// not keep the state; call Onset for that. Detectors use it for
// hysteresis: a level crossed stays crossed until the value falls
// clearly below it. Without a registry it reports false.
func (c Context) Ongoing(key string) bool {
	if c.onsets == nil {
		return false
	}
	_, ok := c.onsets.known[key]
	return ok
}
