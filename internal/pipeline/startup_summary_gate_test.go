package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
)

// without is an edition that has everything e has except id.
type without struct {
	feature.Entitlements
	id feature.ID
}

func (w without) Has(id feature.ID) bool {
	return id != w.id && w.Entitlements.Has(id)
}

// The startup summary and the restored listing follow the
// StartupSummary capability: Pro has it, Community does not, and an
// edition without it starts from a baseline even when it keeps state.
func TestStartupSummaryFollowsTheCatalog(t *testing.T) {
	for _, tc := range []struct {
		name         string
		e            feature.Entitlements
		baseline     bool
		listRestored bool
	}{
		{"pro", feature.ProEntitlements(), false, true},
		{"community", feature.CommunityEntitlements(), true, false},
		{"pro without summary", without{feature.ProEntitlements(),
			feature.StartupSummary}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
			store := stateOfALongRun(t, start)
			h := newHarnessWith(t, start.Add(time.Hour),
				func(d *Dependencies) {
					d.Store, d.Entitlements = store, tc.e
				})

			require.NoError(t, h.engine.restore())

			startup := h.engine.announcer.collect.Startup
			assert.Equal(t, tc.baseline, startup.Baseline)
			assert.Equal(t, tc.listRestored, !startup.WarmAt.IsZero())
		})
	}
}
