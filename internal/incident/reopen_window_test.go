package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pageReturnsAfter resolves the paged node, raises it again gap later and
// returns the first decision of the return.
func pageReturnsAfter(t *testing.T, gap time.Duration) (Incident, Decision) {
	t.Helper()
	r, _ := pagedRig(t)
	first := r.only()
	resolved := resolvePage(t, r, 5*time.Minute)
	raiseAt := resolved + gap
	r.raise(at(raiseAt), nodeDown())
	ds := tickEvery(r, raiseAt, raiseAt+DefaultSettle+time.Minute)
	require.NotEmpty(t, ds)
	return first, ds[0]
}

func TestQuickReturnReopensTheIncident(t *testing.T) {
	first, d := pageReturnsAfter(t, 5*time.Minute)
	assert.Equal(t, Update, d.Action)
	assert.Equal(t, ReasonFailingAgain, d.Reason)
	assert.Equal(t, first.ID, d.Incident.ID, "the outage flaps")
}

func TestLaterOutageIsANewIncidentLinkedToTheOld(t *testing.T) {
	for _, gap := range []time.Duration{
		ReopenWindow + time.Minute, 90 * time.Minute, 3 * time.Hour,
	} {
		first, d := pageReturnsAfter(t, gap)
		assert.Equal(t, Announce, d.Action, gap)
		assert.NotEqual(t, first.ID, d.Incident.ID, gap)
		assert.Equal(t, first.ID, d.Incident.Previous, gap)
	}
}

func TestLaterOutageWithinRepageWindowDoesNotPageAgain(t *testing.T) {
	_, d := pageReturnsAfter(t, 90*time.Minute)
	assert.Equal(t, Notify, d.Incident.Tier, "the page was 90m ago")
}
