package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/feature"
)

// startedHistory relaunches on store with the edition e and starts the
// history as Run does.
func startedHistory(
	t *testing.T, store IncidentStore, e feature.Entitlements,
) *harness {
	t.Helper()
	h := newHarnessWith(t, restartBase, func(d *Dependencies) {
		d.Store, d.Entitlements = store, e
	})
	h.engine.startHistory()
	t.Cleanup(func() { h.engine.storage.history.askStop() })
	return h
}

// Pro keeps the change history across a restart and writes the next
// one.
func TestStartHistoryRestoresChangesOnPro(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	savePreviousRun(t, store, restartBase.Add(-15*time.Hour),
		restartBase.Add(-14*time.Hour))

	h := startedHistory(t, store, feature.ProEntitlements())

	assert.NotEmpty(t, h.engine.deps.Model.Changes(restartDeploy,
		time.Time{}))
	assert.NotNil(t, h.engine.storage.history.writer)
}

// A Community start is a fresh baseline: nothing comes back from disk
// and no history is written for a later restart.
func TestStartHistoryRestoresNothingOnCommunity(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	savePreviousRun(t, store, restartBase.Add(-15*time.Hour),
		restartBase.Add(-14*time.Hour))

	h := startedHistory(t, store, feature.CommunityEntitlements())

	assert.Empty(t, h.engine.deps.Model.Changes(restartDeploy,
		time.Time{}))
	assert.Nil(t, h.engine.storage.history.writer)
	assert.True(t, h.engine.storage.history.stopped())
}
