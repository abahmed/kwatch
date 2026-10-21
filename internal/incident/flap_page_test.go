package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A page that comes back a third time inside RepageWindow tells the pagers
// once, at the page tier, so an outage that keeps flapping has an open
// alert again after the first resolve closed it. The first return is chat
// news only.
func TestThirdReturnOfAPageTellsThePagersAtThePageTier(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	first, raised := failAgain(t, r, resolved, 10*time.Minute)
	require.Equal(t, ReasonFailingAgain, first.Reason)
	assert.Equal(t, Notify, first.Incident.Tier, "the second time: chat")
	resolved = resolvePage(t, r, raised+10*time.Minute)

	second, _ := failAgain(t, r, resolved, 10*time.Minute)

	assert.Equal(t, ReasonFailingAgain, second.Reason)
	assert.Equal(t, 3, second.Incident.RepeatCount)
	assert.Equal(t, Page, second.Incident.Tier, "the third time: pagers")
	assert.True(t, second.Incident.Delivery.PageHeld())
	assert.Equal(t, Notify, r.only().Tier, "the record keeps its hold")
}

// It is one update: with the alert open at the pagers, nothing more is
// sent to them, and the hold is not released into a second page.
func TestFlapperPagesOnceAndItsHoldIsNotReleased(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	_, raised := failAgain(t, r, resolved, 10*time.Minute)
	resolved = resolvePage(t, r, raised+10*time.Minute)
	_, raised = failAgain(t, r, resolved, 10*time.Minute)
	r.m.RecordPaged(r.only().ID, true) // delivery handed the page over

	for _, d := range tickEvery(r, raised+DefaultSettle,
		raised+30*time.Minute) {
		assert.NotEqual(t, Page, d.Incident.Tier,
			"no second page for an alert that is open: %s", d.Reason)
	}
}

// A first resolve of a page that never came back is not a flapper.
func TestSecondReturnOfAPageStaysInChat(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)

	d, _ := failAgain(t, r, resolved, 10*time.Minute)

	assert.Equal(t, Notify, d.Incident.Tier)
}

// heldFlapper is a manager with one flapping incident that is a held
// page, the state pagesFlapper decides on.
func heldFlapper(t *testing.T) (*rig, *Incident) {
	t.Helper()
	r := newRig(t, Config{})
	r.m.model = r.model
	p := &Incident{ID: "held", Root: nodeDown().Entity, State: Flapping,
		Announced: at(0), Delivery: Delivery{pageHeld: true}}
	r.m.incidents[p.ID] = p
	return r, p
}

// A flapper that nothing else covers tells the pagers once.
func TestPagesFlapperTellsThePagersOnce(t *testing.T) {
	r, p := heldFlapper(t)

	assert.True(t, r.m.pagesFlapper(p, Update, ReasonFlapping))
	assert.False(t, r.m.pagesFlapper(p, Update, ReasonFlapping),
		"once per incident")
}

// A held page that is half of an outage another incident already paged
// for does not open a second alert beside it. It may still page when the
// alert beside it has closed.
func TestPagesFlapperStaysQuietBesideAnOpenAlert(t *testing.T) {
	r, p := heldFlapper(t)
	n2 := inventory.CoreID(kube.KindNode, "", "n2")
	beside := &Incident{ID: "open", Root: n2, State: Open,
		Announced: at(10 * time.Second), Delivery: Delivery{paged: true}}
	r.m.incidents[beside.ID] = beside

	assert.False(t, r.m.pagesFlapper(p, Update, ReasonFlapping))
	assert.False(t, p.Delivery.flapPaged, "nothing was told")

	beside.Delivery.ClosePage()
	assert.True(t, r.m.pagesFlapper(p, Update, ReasonFlapping))
}

// Someone who put the acknowledgement annotation on the node knows about
// the outage, also when kwatch has not told the thread about it yet.
func TestPagesFlapperHonoursAnAcknowledgementAnnotation(t *testing.T) {
	r, p := heldFlapper(t)
	setAck(t, r, p.Root, "on it")
	require.Nil(t, p.Ack, "the thread was not told yet")

	assert.False(t, r.m.pagesFlapper(p, Update, ReasonFlapping))
}

// The once-per-incident rule survives a restart: a flapper that already
// told the pagers does not tell them again after kwatch restarts.
func TestFlapperPagedSurvivesARestart(t *testing.T) {
	r, p := heldFlapper(t)
	require.True(t, r.m.pagesFlapper(p, Update, ReasonFlapping))

	back := newRig(t, Config{})
	back.m.Restore(r.m.Export(), time.Time{})

	q := back.m.incidents[p.ID]
	require.NotNil(t, q)
	assert.True(t, q.Delivery.flapPaged)
	assert.False(t, back.m.pagesFlapper(q, Update, ReasonFlapping))
}
