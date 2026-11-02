package inventory

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// More records than one chunk: every one is pruned, and tombstones of
// every chunk are dropped.
func TestPruneKeepingCoversEveryChunk(t *testing.T) {
	m := NewModel(Options{})
	t1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	n := 2*pruneChunk + 7
	for i := range n {
		id := CoreID("pod", "default", fmt.Sprintf("p%d", i))
		_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s", At: t1,
			Entity: id})
		_, _ = m.Apply(Observation{Kind: Changed, Source: "k8s", At: t1,
			Entity: id, Change: Change{At: t1, Actor: "user"}})
		if i%2 == 0 {
			_, _ = m.Apply(Observation{Kind: Gone, Source: "k8s", At: t1,
				Entity: id})
		}
	}

	changes := m.Stats().Changes

	removed := m.Prune(t1.Add(time.Hour))

	assert.Equal(t, changes, removed)
	assert.Equal(t, 0, m.Stats().Changes)
	assert.Equal(t, 0, m.Stats().Tombstones)
	assert.Equal(t, n/2, m.Stats().Entities)
}
