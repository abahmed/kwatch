package incident

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"
)

// toldCausesMax bounds the causes an incident remembers telling.
const toldCausesMax = 8

// causeMemory keeps a cause that flips back and forth from being news.
// The solver may blame a release, then a startup setting, then no rule
// at all, for the same failures, one evaluation after another; each flip
// changed the fingerprint and sent a "material change" about nothing.
//
// The rule: the cause part of the fingerprint stays the one last told
// unless the cause now is one never told for this incident and has held
// for CauseHold, seen on at least two evaluations. The first cause found
// after none was told is news at once. No cause counts as a cause here:
// the explanation may have moved off the cause record (a called backend,
// say), which is news once it holds. A cause told before is never news again by itself; a tier rise, new reasons, impact growth
// and every other part of the fingerprint count as before.
type causeMemory struct {
	// told are hashes of the causes told, oldest first.
	told []string
	// last is the cause of the last told fingerprint; known says it
	// was recorded (an incident from an older kwatch has none until its
	// next message or its restore).
	last  string
	known bool
	// candidate is the cause seen at the latest evaluation, since when,
	// and whether it has held long enough to be news.
	candidate string
	since     time.Time
	held      bool
}

func causeHash(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:hashBytes])
}

// note records the cause seen at one evaluation.
func (c *causeMemory) note(id string, now time.Time) {
	if id != c.candidate || c.since.IsZero() {
		c.candidate, c.since, c.held = id, now, false
		return
	}
	c.held = now.After(c.since) && now.Sub(c.since) >= CauseHold
}

// part is the cause identity the fingerprint reads for id, the cause now.
// A cause found while none is told is news at once, unless it was told
// before: X, then no cause, then X again is a flip, not news.
func (c *causeMemory) part(id string) string {
	if !c.known || id == c.last {
		return id
	}
	told := id != "" && slices.Contains(c.told, causeHash(id))
	if c.last == "" && !told {
		return id
	}
	// No cause is never "told before": what explains the incident then
	// lives elsewhere and may differ from the last time.
	if told ||
		id != c.candidate || !c.held {
		return c.last
	}
	return id
}

// record keeps part as the cause told now.
func (c *causeMemory) record(part string) {
	c.last, c.known = part, true
	h := causeHash(part)
	if slices.Contains(c.told, h) {
		return
	}
	// A new slice: snapshots may share the old one.
	told := append(slices.Clone(c.told), h)
	if len(told) > toldCausesMax {
		told = told[len(told)-toldCausesMax:]
	}
	c.told = told
}

// causePart is the cause identity p's fingerprint reads.
func (p *Incident) causePart() string {
	return p.causes.part(causeIdentity(p))
}
