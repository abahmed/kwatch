package detection

import (
	"slices"
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
	byEntity map[inventory.EntityID]map[string]onsetEntry
}

// onsetEntry is one remembered state and the detector that asked for it
// last, so an evaluation that could not run that detector keeps only its
// states.
type onsetEntry struct {
	at    time.Time
	owner string
}

func newOnsets() *onsets {
	return &onsets{byEntity: map[inventory.EntityID]map[string]onsetEntry{}}
}

// begin starts one evaluation of id: it returns the states known so far.
func (o *onsets) begin(id inventory.EntityID) *onsetScope {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &onsetScope{known: o.byEntity[id], kept: map[string]onsetEntry{}}
}

// finish keeps only the states the evaluation asked for. The states of a
// detector that could not run fully (it panicked, needed a kind not
// synced or found a source stale) stay too: it could not ask for them,
// and forgetting them would restart its waiting period. Every other
// detector's unasked states are forgotten as usual. An evaluation that
// did not finish (done false) keeps every state.
func (o *onsets) finish(
	id inventory.EntityID, scope *onsetScope, unsure []string, done bool,
) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for key, entry := range scope.known {
		if _, asked := scope.kept[key]; asked {
			continue
		}
		if !done || slices.Contains(unsure, entry.owner) {
			scope.kept[key] = entry
		}
	}
	if len(scope.kept) == 0 {
		delete(o.byEntity, id)
		return
	}
	o.byEntity[id] = scope.kept
}

// onsetScope is the onset state of one evaluation of one entity. detector
// names the detector running now, the owner of the states it asks for.
type onsetScope struct {
	known    map[string]onsetEntry
	kept     map[string]onsetEntry
	detector string
}

// running names the detector whose states the next asks belong to.
func (s *onsetScope) running(detector string) {
	if s != nil {
		s.detector = detector
	}
}

func (s *onsetScope) ask(key string, at time.Time) {
	s.kept[key] = onsetEntry{at: at, owner: s.detector}
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
	at := firstSeen
	if entry, ok := c.onsets.known[key]; ok {
		at = entry.at
	}
	c.onsets.ask(key, at)
	return at
}

// Track records value under key for the evaluated entity and returns the
// value the previous evaluation recorded, false when there was none. It
// lets a detector compare what it reads now with what it read before. Like
// an onset, a tracked value is kept only while every evaluation keeps
// asking for it, and without a registry nothing is remembered. Keep keys
// of tracked values apart from those of onsets.
func (c Context) Track(
	key string, value time.Time,
) (previous time.Time, found bool) {
	if c.onsets == nil {
		return time.Time{}, false
	}
	entry, found := c.onsets.known[key]
	c.onsets.ask(key, value)
	return entry.at, found
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
