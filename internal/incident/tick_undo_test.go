package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A tick that panics before its decision reaches the caller is taken
// back whole. The steps before the decision already took the news (the
// acknowledgement, the fix attempt, the page hold, the flap count, the
// timeline note): left in place, the next tick would find nothing new
// to tell.
func TestUndoTickRestoresWhatTheStepsBeforeTheDecisionChanged(t *testing.T) {
	r, start := announcedRig(t)
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p := r.m.incidents[r.m.sortedIDs()[0]]
	seen := &inventory.Change{Actor: "alice"}
	p.attemptSeen = seen
	p.Ack = &Ack{Note: "first"}
	notes, noted := len(p.Timeline), p.sent.noted
	before := snapshotTick(p)

	now := start.Add(time.Minute)
	p.Ack.Note = "edited in place"
	p.Ack = nil
	p.Attempt, p.attemptSeen, p.attemptLate = seen, nil, true
	p.Tier = Digest
	p.note(now, "acknowledgement removed")
	p.Cycles = append(p.Cycles, now)
	p.heldSince = now
	r.m.undoTick(p, before)

	assert.Equal(t, &Ack{Note: "first"}, p.Ack)
	assert.Nil(t, p.Attempt)
	assert.Same(t, seen, p.attemptSeen)
	assert.False(t, p.attemptLate)
	assert.Equal(t, Notify, p.Tier)
	assert.Len(t, p.Timeline, notes)
	assert.Equal(t, noted, p.sent.noted)
	assert.Empty(t, p.Cycles)
	assert.True(t, p.heldSince.IsZero())
}
