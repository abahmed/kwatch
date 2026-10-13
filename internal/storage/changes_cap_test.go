package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The changes bucket round-trips a record through the log, and the
// default policy keeps it for a week and caps its size.
func TestChangeLogRoundTripAndDefaultPolicy(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	log := ChangeLog[string](s)
	at := minute(5)
	want := `{"actor":"argocd","path":"spec.template"}`
	require.NoError(t, log.Append("cluster", at, want))

	got := collect(t, log, "cluster", time.Time{}, time.Time{})

	require.Len(t, got, 1)
	assert.Equal(t, want, got[0])
	policy := DefaultPolicy()
	assert.Equal(t, DefaultChangeRetention, policy.Retention[Changes])
	assert.Equal(t, DefaultChangesCap, policy.ChangesCap)
}

// The changes bucket has its own cap, oldest records first, so a busy
// cluster cannot grow it past the budget.
func TestCompactorChangesCapEvictsOldestChanges(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	entries := make([]Entry[string], 100)
	for i := range entries {
		entries[i] = Entry[string]{
			Entity: fmt.Sprintf("chg-%04d", i), At: minute(i), Value: "c",
		}
	}
	require.NoError(t, ChangeLog[string](s).AppendAll(entries))
	size := entrySize(t, "chg-0000", "c")
	policy := Policy{ChangesCap: 40 * size, Batch: 10}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 60, result.Evicted)
	sizes, err := s.LogicalSize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 40*size, sizes[Changes])
}
