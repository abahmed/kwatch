package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// heldPagedRecords exports an incident whose page went to the pagers
// while its announcement was still held (outage or startup hold).
func heldPagedRecords(t *testing.T) ([]Record, string) {
	t.Helper()
	r, _ := pagedRig(t)
	id := r.only().ID
	r.m.HoldAnnouncement(id)
	r.m.RecordPaged(id, true)
	recs := r.m.Export()
	require.True(t, recs[0].Held)
	require.True(t, recs[0].Paged)
	return recs, id
}

// A page held for a summary reached the pagers before a restart. If its
// failure is gone when it settles again, the pager alert must be closed:
// the resolve goes to the pagers alone, because chat never heard of it.
func TestRestoredHeldPageResolvesPagingOnly(t *testing.T) {
	recs, id := heldPagedRecords(t)
	fresh := newRig(t, Config{})
	fresh.m.Restore(recs, at(15*time.Minute))

	var got []Decision
	for now := 11 * time.Minute; now <= 40*time.Minute; now += 10 * time.Second {
		got = append(got, fresh.tick(at(now))...)
	}

	require.Len(t, got, 1, "one resolve closes the pager alert")
	assert.Equal(t, Resolve, got[0].Action)
	assert.True(t, got[0].Unannounced, "chat never heard of the incident")
	assert.Equal(t, id, got[0].Incident.ID)
	assert.True(t, got[0].Incident.Delivery.OpenAtPagers())
	assert.False(t, fresh.only().CanReopen(),
		"nobody was told, so a return is a new incident")
}

// A held record that never paged still resolves silently.
func TestRestoredHeldUnpagedResolvesSilently(t *testing.T) {
	recs, _ := heldPagedRecords(t)
	recs[0].Paged = false
	fresh := newRig(t, Config{})
	fresh.m.Restore(recs, at(15*time.Minute))
	for now := 11 * time.Minute; now <= 40*time.Minute; now += 10 * time.Second {
		wantNone(t, fresh.tick(at(now)))
	}
	assert.Equal(t, Resolved, fresh.only().State)
}

// A held page whose failure is still there after a restart is announced
// again, but its alert is already open: the announcement goes to chat
// only and the incident stays paged.
func TestRestoredHeldPageDoesNotPageAgain(t *testing.T) {
	recs, id := heldPagedRecords(t)
	fresh := newRig(t, Config{})
	fresh.m.Restore(recs, at(15*time.Minute))
	fresh.raise(at(11*time.Minute), nodeDown())

	var got []Decision
	for now := 11 * time.Minute; now <= 20*time.Minute; now += 10 * time.Second {
		got = append(got, fresh.tick(at(now))...)
	}

	require.NotEmpty(t, got)
	require.Equal(t, Announce, got[0].Action)
	assert.True(t, got[0].PagedAlready, "the alert is already open")
	assert.True(t, fresh.m.PageOpen(id), "and it stays open")
}

func TestFreshAnnouncementIsNotPagedAlready(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), nodeDown())
	ds := tickEvery(r, 0, DefaultSettle)
	require.Len(t, ds, 1)
	assert.False(t, ds[0].PagedAlready)
}
