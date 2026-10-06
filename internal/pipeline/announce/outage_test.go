package announce

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func outageDecision(
	namespace string, i int, opened time.Time,
) incident.Decision {
	return incident.Decision{
		Action: incident.Announce,
		Incident: incident.Incident{
			ID:     fmt.Sprintf("%s-%d", namespace, i),
			Tier:   incident.Notify,
			Opened: opened,
			Root: inventory.EntityID{Kind: kube.KindDeployment,
				Namespace: namespace, Name: fmt.Sprintf("app%d", i)},
		},
	}
}

// Fewer than five failures need to be at least half of a known
// namespace, and three at least.
func TestIsOutageThresholds(t *testing.T) {
	for _, c := range []struct {
		count, total int
		want         bool
	}{
		{5, 0, true}, {4, 0, false}, {2, 3, false}, {3, 6, true},
		{3, 7, false}, {4, 8, true}, {4, 9, false},
	} {
		if got := isOutage(c.count, c.total); got != c.want {
			t.Errorf("isOutage(%d, %d) = %v", c.count, c.total, got)
		}
	}
}

// An incident with a cause or a digest-tier one is never a member, and
// one that opened long before the others is left out of the message.
func TestOutageCandidatesAndWindow(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 20, 0, 0, time.UTC)
	explained := outageDecision("shop", 0, now)
	explained.Incident.Cause = &rootcause.CauseRecord{}
	quiet := outageDecision("shop", 1, now)
	quiet.Incident.Tier = incident.Digest
	update := outageDecision("shop", 2, now)
	update.Action = incident.Update
	for _, d := range []incident.Decision{explained, quiet, update} {
		if isOutageCandidate(d) {
			t.Errorf("%+v must not be a candidate", d.Incident.ID)
		}
	}
	old := outageDecision("shop", 3, now.Add(-11*time.Minute))
	recent := outageDecision("shop", 4, now)

	got := openedTogether([]incident.Decision{old, recent})

	if len(got) != 1 || got[0].Incident.ID != recent.Incident.ID {
		t.Fatalf("members = %+v", got)
	}
}

// Counting the settling incidents of a namespace runs for every candidate
// announcement of a storm, so it must not copy the incidents: it reads
// them in place.
func TestSettlingInDoesNotCopyIncidents(t *testing.T) {
	m := incident.NewManager(incident.Config{}, nil)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	var records []incident.Record
	for i := range 100 {
		d := outageDecision("shop", i, now)
		records = append(records, incident.Record{ID: d.Incident.ID,
			Root: d.Incident.Root, Tier: incident.Notify,
			State: incident.Settling, Opened: now})
	}
	m.Restore(records, time.Time{})
	c := New(Env{Incidents: m})

	assert.Equal(t, 100, c.settlingIn("shop", now))
	assert.Equal(t, 0, c.settlingIn("other", now))
	allocs := testing.AllocsPerRun(10, func() { c.settlingIn("shop", now) })
	assert.Zero(t, allocs, "no incident copies per call")
}
