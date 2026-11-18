package storage

import (
	"slices"
	"time"
)

// A log's writer may hold some of its entities: the objects of the
// incidents that are still open. When a cap evicts from the log, entries
// of entities nobody holds go first, oldest first. Of a held entity the
// middle goes next, thinned: an old stretch keeps its first entry of
// each hour. Its first holdHead entries, its newest holdTail entries and
// the entries at its marks (phase boundaries) go last. So an issue open
// for weeks keeps how it began and how it changed while closed ones lose
// their detail first. The hold is stored in the state bucket, so it
// holds across restarts until the writer replaces it.
const (
	// holdHead and holdTail are the first and newest entries of a held
	// entity kept to the end.
	holdHead = 8
	holdTail = 32
	// maxHeld bounds the held entities and maxHoldMarks the marks of
	// one; past them the rest is held like any entity.
	maxHeld      = 4096
	maxHoldMarks = 32
	// thinAfter is the age past which a held middle keeps one entry per
	// thinStep.
	thinAfter = 24 * time.Hour
	thinStep  = time.Hour
)

// Eviction ranks, lowest evicted first.
const (
	rankFree = iota
	rankThinned
	rankMiddle
	rankKept
)

// LogHold is what a writer holds of one entity: the entity itself, and
// the times of its entries that mark a boundary (a raise, an
// announcement, a resolve).
type LogHold struct {
	Marks []time.Time `json:",omitempty"`
}

func holdKey(b Bucket) string { return "log-hold/" + string(b) }

// Hold replaces what the log's writer holds, by entity. At most maxHeld
// entities and maxHoldMarks marks each (the earliest and the latest
// halves) are kept.
func (l Log[T]) Hold(holds map[string]LogHold) error {
	out := make(map[string]LogHold, min(len(holds), maxHeld))
	keys := make([]string, 0, len(holds))
	for entity := range holds {
		keys = append(keys, entity)
	}
	slices.Sort(keys)
	for _, entity := range keys[:min(len(keys), maxHeld)] {
		out[entity] = LogHold{Marks: boundMarks(holds[entity].Marks)}
	}
	return StateValues[map[string]LogHold](l.store).Put(
		holdKey(l.bucket), out)
}

// boundMarks keeps the earliest and the latest maxHoldMarks/2 marks.
func boundMarks(marks []time.Time) []time.Time {
	marks = slices.Clone(marks)
	slices.SortFunc(marks, func(a, b time.Time) int { return a.Compare(b) })
	if len(marks) <= maxHoldMarks {
		return marks
	}
	half := maxHoldMarks / 2
	return append(marks[:half:half], marks[len(marks)-half:]...)
}

// holds reads what the writer of b holds; none when it holds nothing or
// the record cannot be read.
func (s *Store) holds(b Bucket) map[string]LogHold {
	held, ok, err := StateValues[map[string]LogHold](s).Get(holdKey(b))
	if err != nil || !ok {
		return nil
	}
	return held
}

// holdRanker ranks the entries of one bucket as a scan visits them in
// key order: entity by entity, oldest first. Ranked entries come out
// through emit, the newest holdTail of a held entity only once the
// entity's run ends (flush). It keeps at most holdTail entries.
type holdRanker struct {
	held   map[string]LogHold
	now    time.Time
	emit   func(victim)
	entity string
	hold   LogHold
	isHeld bool
	seen   int
	lastAt time.Time
	tail   []victim
}

// add ranks the entry v of entity.
func (r *holdRanker) add(entity string, v victim) {
	if entity != r.entity {
		r.flush()
		r.entity, r.seen, r.lastAt = entity, 0, time.Time{}
		r.hold, r.isHeld = r.held[entity]
	}
	r.seen++
	if !r.isHeld {
		r.emit(v)
		return
	}
	if r.seen <= holdHead || r.marked(v.at) {
		v.rank = rankKept
		r.emit(v)
		return
	}
	r.tail = append(r.tail, v)
	if len(r.tail) > holdTail {
		r.emit(r.middle(r.tail[0]))
		r.tail = slices.Delete(r.tail, 0, 1)
	}
}

// middle ranks an entry of a held entity that is neither its start, its
// newest nor a mark: an old stretch keeps the first entry of each hour.
func (r *holdRanker) middle(v victim) victim {
	v.rank = rankMiddle
	if r.now.Sub(v.at) > thinAfter {
		step := v.at.Truncate(thinStep)
		if !r.lastAt.IsZero() && step.Equal(r.lastAt) {
			v.rank = rankThinned
		}
		r.lastAt = step
	}
	return v
}

// flush emits the newest entries of the entity whose run ended.
func (r *holdRanker) flush() {
	for _, v := range r.tail {
		v.rank = rankKept
		r.emit(v)
	}
	r.tail = r.tail[:0]
}

func (r *holdRanker) marked(at time.Time) bool {
	return slices.ContainsFunc(r.hold.Marks, at.Equal)
}

// logEntity is the entity part of a log key.
func logEntity(key []byte) string {
	if len(key) < logSuffix+1 {
		return ""
	}
	return string(key[:len(key)-logSuffix-1])
}

// keepMarked is expired that keeps the marked entries of held entities
// past the retention: an issue open longer than the retention keeps how
// it began and changed. Unmarked entries expire as before.
func keepMarked(expired expiredFunc, held map[string]LogHold) expiredFunc {
	if len(held) == 0 {
		return expired
	}
	return func(key, data []byte) bool {
		if !expired(key, data) {
			return false
		}
		hold, ok := held[logEntity(key)]
		at, timed := logTime(key)
		return !ok || !timed || !slices.ContainsFunc(hold.Marks, at.Equal)
	}
}

// Held is what the log's writer last held (Hold); empty when nothing.
func (l Log[T]) Held() (map[string]LogHold, error) {
	held, _, err := StateValues[map[string]LogHold](l.store).Get(
		holdKey(l.bucket))
	return held, err
}
