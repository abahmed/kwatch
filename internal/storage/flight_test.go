package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The flight recording is a ring: bounded by its own cap, oldest first.
func TestCompactorFlightCapEvictsOldestFirst(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	entries := make([]Entry[string], 500)
	for i := range entries {
		entries[i] = Entry[string]{Entity: "obs", At: minute(i),
			Value: fmt.Sprintf("%03d", i)}
	}
	require.NoError(t, FlightLog[string](s).AppendAll(entries))
	size := entrySize(t, "obs", "000")
	policy := Policy{FlightCap: 200 * size, Batch: 10}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 300, result.Evicted)
	sizes, err := s.LogicalSize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 200*size, sizes[Flight])
	got := collect(t, FlightLog[string](s), "obs", time.Time{}, time.Time{})
	assert.Len(t, got, 200)
	assert.Equal(t, "300", got[0], "the oldest went first")
}

func TestCompactorFlightRetentionDropsOldEntries(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	now := clock.Now()
	log := FlightLog[string](s)
	require.NoError(t, log.Append("obs", now.Add(-50*time.Hour), "old"))
	require.NoError(t, log.Append("obs", now.Add(-time.Hour), "new"))

	_, err := NewCompactor(s, DefaultPolicy()).Pass(context.Background())

	require.NoError(t, err)
	got := collect(t, log, "obs", time.Time{}, time.Time{})
	require.Len(t, got, 1)
	assert.Equal(t, "new", got[0])
}

func TestFlightIsEvictableAndBounded(t *testing.T) {
	assert.True(t, isEvictable(Flight))
	p := DefaultPolicy()
	assert.Equal(t, int64(64<<20), p.FlightCap)
	assert.Equal(t, 48*time.Hour, p.Retention[Flight])
	assert.NotEmpty(t, fmt.Sprint(Flight))
}
