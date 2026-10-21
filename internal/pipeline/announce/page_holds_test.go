package announce

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

func pageRecord(id, namespace string) incident.Record {
	return incident.Record{ID: id, Tier: incident.Page,
		State: incident.Settling, Opened: foldNow,
		Root: inventory.CoreID(kube.KindDeployment, namespace, id)}
}

func pageAnnouncement(id, namespace string) incident.Decision {
	return incident.Decision{Action: incident.Announce,
		Reason: incident.ReasonSettled,
		Incident: incident.Incident{ID: id, Tier: incident.Page,
			Opened: foldNow,
			Root:   inventory.CoreID(kube.KindDeployment, namespace, id)}}
}

// pagesIn counts the messages that went to the paging tools alone.
func pagesIn(sent []notification.Message) int {
	n := 0
	for _, m := range sent {
		if m.PagingOnly {
			n++
		}
	}
	return n
}

// One page stands for the problems of its own namespace. A page of
// another namespace is another team's outage: the roll-up of the same
// tick never holds it.
func TestRollupPagesEveryNamespaceOnce(t *testing.T) {
	c, sent := foldHarness(pageRecord("a", "billing"),
		pageRecord("b", "shop"), pageRecord("c", "shop"))
	announcements := []incident.Decision{pageAnnouncement("a", "billing"),
		pageAnnouncement("b", "shop"), pageAnnouncement("c", "shop")}

	_, sentRollup := c.CollectRollup(context.Background(), foldNow,
		announcements)

	require.True(t, sentRollup)
	assert.Equal(t, 2, pagesIn(*sent), "one page each for billing and shop")
	held := map[string]bool{}
	for _, p := range c.env.Incidents.Incidents() {
		held[p.ID] = p.Delivery.PageHeld()
	}
	assert.Equal(t, map[string]bool{"a": false, "b": false, "c": true},
		held, "only the second page of shop is held at notify")
}

// An outage message pages once for the namespace. The other page-tier
// members are held at notify, as the members of a roll-up are: left at
// the page tier their reminders would page seven pagers for an alert
// nobody opened.
func TestOutageListingHoldsThePagesItDidNotSend(t *testing.T) {
	c, sent := foldHarness(pageRecord("a", "shop"),
		pageRecord("b", "shop"), pageRecord("c", "shop"))
	members := []incident.Decision{pageAnnouncement("a", "shop"),
		pageAnnouncement("b", "shop"), pageAnnouncement("c", "shop")}
	msg := c.env.Messages.NamespaceOutage(
		compose.NamespaceOutage{Namespace: "shop"}, members, foldNow)

	c.sendListing(context.Background(), foldNow, members, msg, true)

	assert.Equal(t, 1, pagesIn(*sent), "the namespace is paged once")
	held := 0
	for _, p := range c.env.Incidents.Incidents() {
		if p.Delivery.PageHeld() {
			held++
			assert.Equal(t, incident.Notify, p.Tier)
		}
	}
	assert.Equal(t, 2, held, "the other two page-tier members are held")
}
