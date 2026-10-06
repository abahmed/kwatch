package scorecard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/audit"
)

// A roll-up is one message however many incidents it names: the member
// lines it carries are not messages people received.
func TestScoreCountsOnlyMessagesPeopleReceived(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{{Timestamp: now, Incident: "",
		Action: audit.ActionCreate, Reason: "roll-up",
		CauseState: audit.CauseKnown}}
	for _, id := range []string{"a", "b", "c", "d"} {
		entries = append(entries, audit.Entry{Timestamp: now,
			Incident: id, Action: audit.ActionCreate,
			Delivery: "roll-up", CauseState: audit.CauseUnknown})
	}
	entries = append(entries,
		audit.Entry{Timestamp: now.Add(time.Minute), Incident: "a",
			Action: audit.ActionUpdate, Delivery: "dropped"},
		audit.Entry{Timestamp: now.Add(time.Minute), Incident: "b",
			Action: audit.ActionResolved, Delivery: "unannounced"},
		audit.Entry{Timestamp: now.Add(time.Minute), Incident: "c",
			Action: audit.ActionCreate, Delivery: "paging",
			Previous: "old", CauseState: audit.CauseKnown})

	report := Score(entries)

	assert.Equal(t, 2, report.Notifications, "the roll-up and one page")
	assert.Equal(t, 0, report.Updates, "the dropped update was never sent")
	assert.Equal(t, 0, report.UnknownCause)
	assert.Equal(t, 2, report.PeakPerHour)
	assert.Equal(t, 1, report.MaxPerIncident)
	assert.Equal(t, 1, report.Recreated, "the lifecycle still counts")
}

// A resolve nobody received still ends its incident: announcing it again
// is a re-creation, and two resolves in a row are a repeated recovery.
func TestScoreLifecycleIgnoresDeliveryOfCarriedEntries(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	report := Score([]audit.Entry{
		{Timestamp: now, Incident: "a", Action: audit.ActionCreate},
		{Timestamp: now, Incident: "a", Action: audit.ActionResolved,
			Delivery: "unannounced"},
		{Timestamp: now, Incident: "a", Action: audit.ActionResolved},
		{Timestamp: now, Incident: "a", Action: audit.ActionCreate},
	})
	assert.Equal(t, 1, report.Recreated)
	assert.Equal(t, 1, report.RepeatedResolves)
}

// One decision can be logged twice: for the pagers and for chat. It is
// one message of the incident, and the pair must not read as a repeated
// resolve or announcement either.
func TestScoreCountsAPagedDecisionOnce(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	report := Score([]audit.Entry{
		{Timestamp: now, Incident: "a", Action: audit.ActionCreate,
			Revision: 1, Delivery: "paging"},
		{Timestamp: now, Incident: "a", Action: audit.ActionCreate,
			Revision: 1, Delivery: "chat"},
		{Timestamp: now, Incident: "a", Action: audit.ActionResolved,
			Revision: 2, Delivery: "paging"},
		{Timestamp: now, Incident: "a", Action: audit.ActionResolved,
			Revision: 2},
		{Timestamp: now, Incident: "b", Action: audit.ActionCreate,
			Revision: 1, Delivery: "paging"},
	})

	assert.Equal(t, 3, report.Notifications,
		"a's announce, a's resolve and b's page")
	assert.Equal(t, 0, report.RepeatedResolves)
	assert.Equal(t, 2, report.MaxPerIncident)
}
