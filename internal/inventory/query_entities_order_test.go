package inventory

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Callers rely on Entities being sorted by string form and do not sort
// again (crash log rounds, Service probing).
func TestModelEntitiesAreSortedByStringForm(t *testing.T) {
	m := NewModel(Options{})
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for _, name := range []string{"zeta", "alpha", "mid", "beta", "omega"} {
		_, err := m.Apply(Observation{
			Kind: Observed, Source: "k", At: at,
			Entity: CoreID("service", fmt.Sprint("ns-", name), name),
		})
		assert.NoError(t, err)
	}

	ids := m.Entities("service")

	assert.Len(t, ids, 5)
	assert.True(t, sort.SliceIsSorted(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	}), "got %v", ids)
}
