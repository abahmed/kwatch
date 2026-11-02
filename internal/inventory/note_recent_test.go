package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// sightings records notes without an origin, a minute apart, and returns
// how many occurrences the model counts.
func sightings(t *testing.T, counts ...int) int {
	t.Helper()
	m := NewModel(Options{})
	id := CoreID("order", "ops", "web")
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	for i, count := range counts {
		at := start.Add(time.Duration(i) * time.Minute)
		m.Apply(Observation{Kind: Noted, Source: "test", At: at, Entity: id,
			Note: Note{At: at, Source: "s", Reason: "Failed", Count: count}})
	}
	notes := m.Notes(id, start)
	assert.Len(t, notes, 1)
	return notes[0].CountSince(start)
}

func TestNoteSightingCountingUpIsOneOccurrence(t *testing.T) {
	assert.Equal(t, 2, sightings(t, 1, 2))
}

func TestNoteSeparateSightingsAddUp(t *testing.T) {
	assert.Equal(t, 3, sightings(t, 1, 1, 1))
}
