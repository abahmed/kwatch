package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/unexplained"
)

var unexplainedNow = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)

func warningNote(reason, origin string, count int, at time.Time,
) inventory.Observation {
	return inventory.Observation{Kind: inventory.Noted, At: at,
		Entity: podEnt, Note: inventory.Note{Reason: reason, Warning: true,
			Origin: origin, Count: count, At: at,
			Message: "kubelet: " + reason + " happened"}}
}

func newTracker() *unexplainedTracker {
	return newUnexplainedTracker(func() time.Time { return unexplainedNow })
}

func TestUnknownWarningEventsAreCountedEveryOccurrence(t *testing.T) {
	u := newTracker()
	before := metrics.DefaultRegistry().Unexplained[0].Load()

	u.observeNote(warningNote("BrandNewReason", "e1", 1, unexplainedNow))
	u.observeNote(warningNote("BrandNewReason", "e1", 4, unexplainedNow))
	u.observeNote(warningNote("BrandNewReason", "e2", 2, unexplainedNow))
	// The same Event delivered again adds nothing.
	u.observeNote(warningNote("BrandNewReason", "e2", 2, unexplainedNow))

	got := u.report(unexplainedNow)
	assert.Equal(t, 6, got.Events)
	require.Len(t, got.Top, 1)
	assert.Equal(t, "BrandNewReason", got.Top[0].Reason)
	assert.Equal(t, string(detection.ModeUnusualEvent), got.Top[0].Mode)
	assert.Contains(t, got.Top[0].Sample, "BrandNewReason happened")
	after := metrics.DefaultRegistry().Unexplained[0].Load()
	assert.Equal(t, int64(6), after-before)
}

func TestKnownAndNormalEventsAreNotUnexplained(t *testing.T) {
	u := newTracker()
	u.observeNote(warningNote("FailedMount", "e1", 3, unexplainedNow))
	u.observeNote(warningNote("BackOff", "e2", 3, unexplainedNow))
	normal := warningNote("BrandNewReason", "e3", 1, unexplainedNow)
	normal.Note.Warning = false
	u.observeNote(normal)

	assert.Zero(t, u.report(unexplainedNow).Events)
}

func unclearDecision(id string, unclear bool) incident.Decision {
	f := detection.Finding{Entity: podEnt, Reason: "CrashLoopBackOff",
		Severity: detection.Critical, Mode: detection.ModeCrashLoop,
		Evidence: []detection.Evidence{{Label: "error", Value: "exit 3"}}}
	return incident.Decision{Action: incident.Update,
		Incident: incident.Incident{ID: id, Root: podEnt,
			CauseUnclear: unclear,
			Members: map[detection.Key]detection.Finding{
				{Entity: podEnt, Reason: f.Reason}: f}}}
}

func TestIncidentsWithNoClearCauseAreCountedOnce(t *testing.T) {
	u := newTracker()
	u.observeDecisions(unexplainedNow, []incident.Decision{
		unclearDecision("a", true), unclearDecision("b", false)})
	u.observeDecisions(unexplainedNow, []incident.Decision{
		unclearDecision("a", true)})

	got := u.report(unexplainedNow)

	assert.Equal(t, 1, got.Incidents)
	require.Len(t, got.Top, 1)
	assert.Equal(t, "incident", got.Top[0].Kind)
	assert.Equal(t, string(detection.ModeCrashLoop), got.Top[0].Mode)
	assert.Equal(t, "CrashLoopBackOff", got.Top[0].Reason)
	assert.Equal(t, "exit 3", got.Top[0].Sample)
}

// A restart lists the live Events again: the ones the last run counted
// are not counted a second time, new ones are.
func TestUnexplainedCountsSurviveARestart(t *testing.T) {
	disk := NewIncidentStore(openTempStore(t))
	first := newTracker()
	var saved json.RawMessage
	first.save = func(raw json.RawMessage) { saved = raw }
	old := unexplainedNow.Add(-time.Minute)
	first.observeNote(warningNote("BrandNewReason", "e1", 2, old))
	first.observeDecisions(unexplainedNow, []incident.Decision{
		unclearDecision("a", true)})
	require.NotNil(t, saved)
	require.NoError(t, disk.(unexplainedStore).SaveUnexplained(saved))

	second := newTracker()
	second.restore(disk)
	second.observeNote(warningNote("BrandNewReason", "e1", 2, old))
	second.observeNote(warningNote("BrandNewReason", "e9", 1,
		unexplainedNow.Add(time.Minute)))
	second.observeDecisions(unexplainedNow, []incident.Decision{
		unclearDecision("a", true)})

	got := second.report(unexplainedNow.Add(time.Minute))
	assert.Equal(t, 3, got.Events)
	assert.Equal(t, 1, got.Incidents)
}

func TestUnexplainedWritesArePacedAndStayBounded(t *testing.T) {
	u := newUnexplainedTracker(func() time.Time { return unexplainedNow })
	writes := 0
	u.save = func(json.RawMessage) { writes++ }
	for i := range 3 * maxOrigins {
		u.observeNote(warningNote("BrandNewReason", "e"+string(rune(i)),
			1, unexplainedNow))
	}
	assert.Equal(t, 1, writes, "one write per pace, the rest wait")
	assert.LessOrEqual(t, len(u.last), maxOrigins)
	u.flush()
	assert.Equal(t, 2, writes)
	assert.LessOrEqual(t, len(u.counts.Summary(unexplainedNow, 100).Top),
		unexplained.MaxRows+1)
}
