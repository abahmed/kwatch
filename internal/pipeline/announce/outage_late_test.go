package announce

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// settlingRecords are n incidents of shop that opened nine minutes ago and
// are still settling: with a candidate they make five, an outage forming.
func settlingRecords(n int) []incident.Record {
	var records []incident.Record
	for i := range n {
		name := fmt.Sprintf("p%d", i)
		records = append(records, incident.Record{
			ID:   "pending-" + name,
			Root: inventory.CoreID(kube.KindDeployment, "shop", name),
			Tier: incident.Notify, State: incident.Settling,
			Opened: foldNow.Add(-9 * time.Minute)})
	}
	return records
}

// sentOutage sends the outage of five workloads of shop and returns the
// collector and its messages. extra are more incidents the manager holds.
func sentOutage(
	t *testing.T, extra ...incident.Record,
) (*Collector, *[]notification.Message) {
	t.Helper()
	c, sent := foldHarness(append(settlingRecords(4), extra...)...)
	c.env.WithChanges = func(d incident.Decision, _ time.Time,
	) incident.Decision {
		return d
	}
	ctx := context.Background()
	var first []incident.Decision
	for i := range 5 {
		first = append(first,
			outageDecision("shop", i, foldNow.Add(-8*time.Minute)))
	}
	rest, _ := c.CollectOutages(ctx, foldNow, first)
	require.Empty(t, rest, "the outage is forming: all held")
	_, flushed := c.CollectOutages(ctx, foldNow.Add(31*time.Second), nil)
	require.True(t, flushed, "one message names the namespace")
	require.Len(t, *sent, 1)
	return c, sent
}

// The sixth workload of the namespace fails two minutes after its outage
// was told. Alone it is too few to be an outage, but it is not news of its
// own either: it joins the outage, which is sent again naming it, instead
// of being announced by itself.
func TestLateFailureOfANamespaceOutageJoinsTheOutage(t *testing.T) {
	c, sent := sentOutage(t)
	late := outageDecision("shop", 5, foldNow.Add(2*time.Minute))

	rest, flushed := c.CollectOutages(context.Background(),
		foldNow.Add(2*time.Minute), []incident.Decision{late})

	assert.Empty(t, rest, "not announced alone")
	assert.True(t, flushed)
	require.Len(t, *sent, 2, "one more outage message")
	assert.Equal(t, []string{late.Incident.ID}, (*sent)[1].Members,
		"it lists the new member; the others have their listing")
}

// Past the outage window the namespace has no outage to join: a failure
// alone is announced alone.
func TestFailureLongAfterAnOutageIsAnnouncedAlone(t *testing.T) {
	c, sent := sentOutage(t)
	late := outageDecision("shop", 5, foldNow.Add(20*time.Minute))

	rest, flushed := c.CollectOutages(context.Background(),
		foldNow.Add(20*time.Minute), []incident.Decision{late})

	assert.Equal(t, []incident.Decision{late}, rest)
	assert.False(t, flushed)
	assert.Len(t, *sent, 1)
}

// Another namespace is not part of it.
func TestFailureOfAnotherNamespaceDoesNotJoinTheOutage(t *testing.T) {
	c, _ := sentOutage(t)
	other := outageDecision("billing", 0, foldNow.Add(2*time.Minute))

	rest, _ := c.CollectOutages(context.Background(),
		foldNow.Add(2*time.Minute), []incident.Decision{other})

	assert.Equal(t, []incident.Decision{other}, rest)
}

// shopRecord is an incident of workload app<i> of shop in state and tier.
func shopRecord(
	i int, tier incident.Tier, state incident.State,
) incident.Record {
	return incident.Record{ID: fmt.Sprintf("shop-%d", i), Tier: tier,
		State: state, Opened: foldNow.Add(-8 * time.Minute),
		Announced: foldNow, PagedKnown: true,
		Root: inventory.EntityID{Kind: kube.KindDeployment,
			Namespace: "shop", Name: fmt.Sprintf("app%d", i)}}
}

// The outage paged and its alert is open. The sixth workload fails late at
// the page tier: the namespace was paged for the outage, so the late page
// is held at notify, as the pages that follow it are, and no second alert
// opens for the same outage.
func TestLatePageTierFailureOfAnOpenOutageIsNotPagedAgain(t *testing.T) {
	paged := shopRecord(0, incident.Page, incident.Open)
	paged.Paged = true
	c, sent := sentOutage(t, paged,
		shopRecord(5, incident.Page, incident.Open))
	late := outageDecision("shop", 5, foldNow.Add(2*time.Minute))
	late.Incident.Tier = incident.Page

	rest, flushed := c.CollectOutages(context.Background(),
		foldNow.Add(2*time.Minute), []incident.Decision{late})

	assert.Empty(t, rest)
	assert.True(t, flushed)
	assert.Zero(t, pagesIn(*sent), "no second page for the same outage")
	assert.False(t, c.env.Incidents.PageOpen(late.Incident.ID))
	held := false
	for _, p := range c.env.Incidents.Incidents() {
		if p.ID == late.Incident.ID {
			held = p.Delivery.PageHeld()
		}
	}
	assert.True(t, held, "the late page is held at notify")
}

// When no page of the outage is open (it never paged, or its alert was
// closed), a late page-tier failure pages as it would anywhere else.
func TestLatePageTierFailureOfAnOutageWithoutAPagePages(t *testing.T) {
	c, sent := sentOutage(t, shopRecord(5, incident.Page, incident.Open))
	late := outageDecision("shop", 5, foldNow.Add(2*time.Minute))
	late.Incident.Tier = incident.Page

	c.CollectOutages(context.Background(), foldNow.Add(2*time.Minute),
		[]incident.Decision{late})

	assert.Equal(t, 1, pagesIn(*sent))
}
