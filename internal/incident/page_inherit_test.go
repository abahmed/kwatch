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

// The reasoning splits one outage over two roots: a paged incident of
// the Deployment is open, and a failure of the Service in front of it
// opens an incident of the Service. The people paged for the outage are not
// paged again for the other half of it: the new incident is announced
// to the chat and the thread, and holds back the page.
func TestSplitOfAPagedIncidentIsAnnouncedWithoutPaging(t *testing.T) {
	r := newRig(t, Config{})
	_, svc, crash := storyRig(t, r)
	crash = sig(crash.Entity, reasons.CoreDNSUnavailable, detection.Critical)
	r.raise(at(0), crash)
	ds := r.tick(at(DefaultSettle))
	require.Len(t, ds, 1)
	require.Equal(t, Page, ds[0].Incident.Tier)
	id := ds[0].Incident.ID
	r.m.RecordPaged(id, true)

	other := sig(svc, reasons.APIServerUnavailable,
		detection.Critical)
	r.raise(at(4*time.Minute), other)
	var got []Decision
	for now := 4 * time.Minute; now < 10*time.Minute; now += 10 * time.Second {
		got = append(got, r.tick(at(now))...)
	}

	require.Len(t, got, 1)
	assert.Equal(t, Announce, got[0].Action)
	assert.NotEqual(t, id, got[0].Incident.ID, "a split, not a join")
	assert.NotEqual(t, Page, got[0].Incident.Tier, "no second page")
}

// A failure of an unrelated workload still pages.
func TestUnrelatedFailureStillPagesWhileAnotherPageIsOpen(t *testing.T) {
	r, _ := pagedRig(t)
	other := sig(entity(kube.KindPod, "dns"), reasons.CoreDNSUnavailable,
		detection.Critical)
	r.raise(at(4*time.Minute), other)
	var got []Decision
	for now := 4 * time.Minute; now < 10*time.Minute; now += 10 * time.Second {
		got = append(got, r.tick(at(now))...)
	}
	require.Len(t, got, 1)
	assert.Equal(t, Page, got[0].Incident.Tier)
}
