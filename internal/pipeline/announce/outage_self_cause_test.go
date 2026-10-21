package announce

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A workload that has a Service names itself as its own cause (the "self"
// row): that ties it to no other failure, so it may be one of a
// namespace's outage. Only a cause beyond the workload keeps it out.
func TestWorkloadBlamedForItselfIsAnOutageCandidate(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 20, 0, 0, time.UTC)
	own := outageDecision("shop", 0, now)
	own.Incident.Cause = &rootcause.CauseRecord{Rule: "self"}
	other := outageDecision("shop", 1, now)
	other.Incident.Cause = &rootcause.CauseRecord{Rule: "node-not-ready"}

	assert.True(t, isOutageCandidate(own))
	assert.False(t, isOutageCandidate(other))
}

// The hold counts such incidents when it asks how many more of the
// namespace are still settling.
func TestSettlingIncidentsBlamedForThemselvesCountForTheOutage(t *testing.T) {
	m := incident.NewManager(incident.Config{}, nil)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	var records []incident.Record
	for i := range 6 {
		d := outageDecision("shop", i, now)
		records = append(records, incident.Record{ID: d.Incident.ID,
			Root: d.Incident.Root, Tier: incident.Notify,
			State: incident.Settling, Opened: now,
			Cause: &rootcause.CauseRecord{Rule: "self"}})
	}
	m.Restore(records, time.Time{})
	c := New(Env{Incidents: m})

	assert.Equal(t, 6, c.settlingIn("shop", now))
}
