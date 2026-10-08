package announce

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// A problem is listed in full the first time a day, then only counted as
// unchanged, and in full again the next day.
func TestOngoingIsListedInFullOnceADay(t *testing.T) {
	c, _ := foldHarness(demotedRecord("a"), demotedRecord("b"))
	items := c.ongoingProblems(foldNow)
	require.Len(t, items, 2)

	changed, unchanged := c.splitOngoing(items, foldNow)
	assert.Len(t, changed, 2, "nothing was listed yet")
	assert.Empty(t, unchanged)

	for _, item := range changed {
		c.markShown(item.p, foldNow)
	}
	changed, unchanged = c.splitOngoing(items, foldNow.Add(5*time.Hour))
	assert.Empty(t, changed)
	assert.Len(t, unchanged, 2)

	changed, _ = c.splitOngoing(items, foldNow.Add(24*time.Hour))
	assert.Len(t, changed, 2, "the next day lists them again")
}

// A problem that changed since it was listed is listed in full again.
func TestOngoingThatChangedIsListedInFullAgain(t *testing.T) {
	c, _ := foldHarness(demotedRecord("a"), demotedRecord("b"))
	items := c.ongoingProblems(foldNow)
	for _, item := range items {
		item.p.Members = failingMember(item.p)
		c.markShown(item.p, foldNow)
	}
	items[0].p.Members = failingMember(items[0].p)
	items[0].p.State = incident.Flapping
	items[1].p.Members = failingMember(items[1].p)

	changed, unchanged := c.splitOngoing(items, foldNow.Add(time.Hour))

	require.Len(t, changed, 1)
	assert.Equal(t, items[0].p.ID, changed[0].p.ID)
	assert.Len(t, unchanged, 1)
}

// failingMember is the one finding of a problem that still fails.
func failingMember(p incident.Incident) map[detection.Key]detection.Finding {
	f := detection.Finding{Entity: p.Root, Reason: "FailedDeployModel"}
	return map[detection.Key]detection.Finding{f.Key(): f}
}

// A problem whose findings are gone is about to resolve: it is not news.
func TestOngoingWithoutFindingsIsUnchanged(t *testing.T) {
	c, _ := foldHarness(demotedRecord("a"))
	items := c.ongoingProblems(foldNow)
	items[0].p.Members = failingMember(items[0].p)
	c.markShown(items[0].p, foldNow)
	items[0].p.Members = nil

	_, unchanged := c.splitOngoing(items, foldNow.Add(time.Hour))

	assert.Len(t, unchanged, 1)
}
