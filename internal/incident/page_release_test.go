package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A page that came back and stays down is a real second outage: after
// the page settle and the base hold it pages once more, and only once.
func TestHeldRepeatThatKeepsFailingPagesOnce(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	d, _ := failAgain(t, r, resolved, 10*time.Minute)
	assert.Equal(t, Notify, d.Incident.Tier, "the return itself is held")

	start := r.only().Opened
	wantNone(t, r.tick(start.Add(DefaultPageSettle+DefaultHold-time.Second)))
	paged := r.tick(start.Add(DefaultPageSettle + DefaultHold))
	wantAction(t, paged, Update, ReasonMaterialChange)
	assert.Equal(t, Page, paged[0].Incident.Tier)
	assert.False(t, paged[0].Incident.Delivery.PageHeld())

	later := start.Add(DefaultPageSettle + DefaultHold + time.Minute)
	wantNone(t, r.tick(later))
}

// A return that recovers again before the hold is a flap, and a flap
// is not paged again.
func TestHeldRepeatThatRecoversStaysHeld(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	_, raiseAt := failAgain(t, r, resolved, 10*time.Minute)

	r.clear(at(raiseAt+time.Minute), nodeDown())
	r.tick(at(raiseAt + time.Minute + 10*time.Second))
	r.raise(at(raiseAt+2*time.Minute), nodeDown())
	for now := raiseAt + 2*time.Minute; now < raiseAt+4*time.Minute; now +=
		10 * time.Second {
		for _, d := range r.tick(at(now)) {
			require.NotEqual(t, Page, d.Incident.Tier, "a flap never pages")
		}
	}
}

// Someone who acknowledged the incident knows about it: no page.
func TestAcknowledgedHeldRepeatDoesNotPageAgain(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	_, raiseAt := failAgain(t, r, resolved, 10*time.Minute)
	setAck(t, r, nodeID(), "replacing the node")

	start := at(raiseAt)
	for _, d := range r.tick(start.Add(DefaultPageSettle + DefaultHold +
		time.Minute)) {
		assert.NotEqual(t, Page, d.Incident.Tier)
	}
}
