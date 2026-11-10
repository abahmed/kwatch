package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// In a cluster set as not production nothing pages: a page is capped
// at Needs attention, and the alert level says so plainly.
func TestNonProductionNeverPages(t *testing.T) {
	r := newRig(t, Config{NonProduction: true})
	r.raise(at(0), sig(entity(kube.KindNode, "n1"), reasons.NodeNotReady,
		detection.Critical))

	ds := r.tick(at(DefaultSettle))

	require.Len(t, ds, 1)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
	why := AlertLevelText(ds[0].Incident.Journal.Level)
	assert.Contains(t, why, NonProductionText)
	assert.True(t, r.m.NonProduction())
}

// The same failure pages in production, the default.
func TestProductionPages(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), sig(entity(kube.KindNode, "n1"), reasons.NodeNotReady,
		detection.Critical))

	ds := r.tick(at(DefaultSettle))

	require.Len(t, ds, 1)
	assert.Equal(t, Page, ds[0].Incident.Tier)
	assert.False(t, r.m.NonProduction())
}

// restoredPage restores an announced page, open at the pagers, into a
// manager with cfg, and brings its failure back.
func restoredPage(t *testing.T, cfg Config) *rig {
	t.Helper()
	old, _ := pagedRig(t)
	old.m.RecordPaged(old.only().ID, true)
	recs := old.m.Export()
	require.Equal(t, Page, recs[0].Tier)
	require.True(t, recs[0].Paged)

	r := newRig(t, cfg)
	r.m.Restore(recs, at(10*time.Minute))
	r.raise(at(10*time.Minute), nodeDown())
	return r
}

// A page from before the cluster was set as not production (or paging
// was turned off) does not stay a page after the restart. It falls to
// Needs attention without a message, and its six-hour page reminder is
// not sent.
func TestRestoredPageFallsSilentlyWhereNothingPages(t *testing.T) {
	for name, cfg := range map[string]Config{
		"non-production": {NonProduction: true},
		"paging off":     {NoPaging: true},
	} {
		t.Run(name, func(t *testing.T) {
			r := restoredPage(t, cfg)

			for _, d := range tickEvery(r, 10*time.Minute,
				10*time.Minute+PageRemindAfter+time.Minute) {
				assert.NotEqual(t, Page, d.Incident.Tier, "%s", d.Reason)
				assert.NotEqual(t, ReasonReminder, d.Reason)
			}
			wantNone(t, tickEvery(r, 10*time.Minute, 40*time.Minute))
			assert.Equal(t, Notify, r.only().Tier)
		})
	}
}

// The alert the page opened at the pagers is still open there: the
// resolve closes it.
func TestRestoredPageStillClosesAtThePagersWhereNothingPages(
	t *testing.T,
) {
	for name, cfg := range map[string]Config{
		"non-production": {NonProduction: true},
		"paging off":     {NoPaging: true},
	} {
		t.Run(name, func(t *testing.T) {
			r := restoredPage(t, cfg)
			wantNone(t, tickEvery(r, 10*time.Minute, 20*time.Minute))
			require.Equal(t, Notify, r.only().Tier)
			id := r.only().ID
			require.True(t, r.m.PageOpen(id), "still open at the pagers")

			r.clear(at(20*time.Minute), nodeDown())
			var resolve *Decision
			for _, d := range tickEvery(r, 20*time.Minute, time.Hour) {
				if d.Action == Resolve {
					resolve = &d
				}
			}

			require.NotNil(t, resolve, "the resolve is sent")
			assert.True(t, resolve.Incident.Delivery.OpenAtPagers(),
				"and it goes to the pagers, to close the alert")
			assert.False(t, resolve.Unannounced)
		})
	}
}
