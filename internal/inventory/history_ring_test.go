package inventory

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var ringBase = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// editAt is a Deployment edit, so it lands in the cluster-wide ring.
func editAt(i int) Change {
	return Change{
		Entity: CoreID("deployment", "shop", fmt.Sprintf("d%d", i%50)),
		At:     ringBase.Add(time.Duration(i) * time.Second),
		Fields: []FieldChange{{Path: "spec.image", Before: "a", After: "b"}},
	}
}

func fullHistory() *history {
	h := newHistory()
	for i := 0; i < DefaultMaxRecentChanges; i++ {
		h.addChange(editAt(i))
	}
	return &h
}

func TestHistoryFullRingCostStaysFlat(t *testing.T) {
	h := fullHistory()
	const calls = 2000
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < calls; i++ {
		h.addChange(editAt(DefaultMaxRecentChanges + i))
	}
	runtime.ReadMemStats(&after)
	perCall := (after.TotalAlloc - before.TotalAlloc) / calls
	// Copying the whole ring on every change costs a ring's worth of
	// entries (hundreds of KiB); the amortized cost must stay far below.
	assert.Less(t, perCall, uint64(16<<10),
		"bytes allocated per change when the ring is full")
}

func TestHistoryFullRingKeepsMemoryBounded(t *testing.T) {
	h := fullHistory()
	for i := 0; i < 8*DefaultMaxRecentChanges; i++ {
		h.addChange(editAt(DefaultMaxRecentChanges + i))
		if len(h.recent) != DefaultMaxRecentChanges ||
			cap(h.recent) > 2*DefaultMaxRecentChanges {
			t.Fatalf("ring holds %d entries in %d slots, limit %d",
				len(h.recent), cap(h.recent), DefaultMaxRecentChanges)
		}
	}
}

func TestHistoryFullRingEvictsTheOldestInOrder(t *testing.T) {
	h := fullHistory()
	const extra = 3*DefaultMaxRecentChanges + 7
	for i := 0; i < extra; i++ {
		h.addChange(editAt(DefaultMaxRecentChanges + i))
	}
	assert.Equal(t, uint64(extra), h.evictions)
	assert.Equal(t, uint64(extra+1), h.recent[0].seq)
	for i := 1; i < len(h.recent); i++ {
		assert.Equal(t, h.recent[i-1].seq+1, h.recent[i].seq)
	}
	assert.Equal(t, uint64(DefaultMaxRecentChanges+extra),
		h.recent[len(h.recent)-1].seq)
}
