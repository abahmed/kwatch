package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

func outageDecision(
	namespace string, i int, opened time.Time,
) incident.Decision {
	d := announcement(fmt.Sprintf("%s-%d", namespace, i))
	d.Incident.Tier = incident.Notify
	d.Incident.Opened = opened
	d.Incident.Root = inventory.EntityID{Kind: kube.KindDeployment,
		Namespace: namespace, Name: fmt.Sprintf("app%d", i)}
	return d
}

func outageDecisions(
	namespace string, n int, opened time.Time,
) []incident.Decision {
	var out []incident.Decision
	for i := range n {
		out = append(out, outageDecision(namespace, i, opened))
	}
	return out
}

// outageEngine is an engine with a hold already open for shop, as when
// other incidents of the namespace are still settling.
func outageEngine(
	t *testing.T, now time.Time,
) (*Engine, *[]notification.Message) {
	e, seen := rollupHarness(t, now)
	e.announcer.collect.Outages["shop"] = &announce.OutageHold{Since: now}
	return e, seen
}

// Five unrelated failures in one namespace go as one message whose
// members thread under it; another namespace's failure is left alone.
func TestOutageGroupsAFailingNamespace(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	e, seen := outageEngine(t, now)
	in := outageDecisions("shop", 5, now.Add(-3*time.Minute))
	in = append(in, outageDecision("other", 0, now))

	rest, sent := e.announcer.collect.CollectOutages(context.Background(), now, in)

	if !sent || len(rest) != 1 || rest[0].Incident.Root.Namespace != "other" {
		t.Fatalf("only the other namespace passes through: %d", len(rest))
	}
	msg := (*seen)[0]
	if !strings.HasPrefix(msg.Key, notification.RollupKeyPrefix) ||
		len(msg.Members) != 5 ||
		!strings.Contains(msg.Title, "shop: 5 workloads failing since 10:02") {
		t.Fatalf("outage message = %+v", msg)
	}
	if len(*seen) != 6 {
		t.Fatalf("want outage + 5 carried, got %d", len(*seen))
	}
	listings := e.announcer.collect.Startup.Summary.Rollups
	if len(listings) != 1 || len(listings[0].Incidents) != 5 {
		t.Fatalf("outage not remembered as a roll-up: %+v", listings)
	}
	if len(e.announcer.collect.Outages) != 0 {
		t.Fatal("the hold must be gone once sent")
	}
}

// Without an outage forming, announcements pass through untouched.
func TestOutageLeavesALoneFailureAlone(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	in := outageDecisions("shop", 2, now)

	rest, sent := e.announcer.collect.CollectOutages(context.Background(), now, in)

	if sent || len(rest) != 2 || len(*seen) != 0 ||
		len(e.announcer.collect.Outages) != 0 {
		t.Fatalf("sent=%v rest=%d seen=%d", sent, len(rest), len(*seen))
	}
}

// A hold that ends with too few incidents hands them back to be
// announced one by one.
func TestOutageHoldThatStaysSmallIsReleased(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	e, seen := outageEngine(t, now)

	rest, sent := e.announcer.collect.CollectOutages(context.Background(), now,
		outageDecisions("shop", 2, now))

	if sent || len(rest) != 2 || len(*seen) != 0 {
		t.Fatalf("sent=%v rest=%d seen=%d", sent, len(rest), len(*seen))
	}
}

// A held incident that resolves before the message goes is left out of
// it, and an update only refreshes what the message will say.
func TestOutageFoldsUpdatesAndResolves(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	e, _ := outageEngine(t, now)
	h := e.announcer.collect.Outages["shop"]
	h.Held = outageDecisions("shop", 3, now)
	update := outageDecision("shop", 1, now)
	update.Action = incident.Update
	resolve := outageDecision("shop", 2, now)
	resolve.Action = incident.Resolve

	rest, _ := e.announcer.collect.HoldOutage(context.Background(), now, update),
		e.announcer.collect.HoldOutage(context.Background(), now, resolve)

	if !rest || len(h.Held) != 2 {
		t.Fatalf("held = %d, want the resolved one dropped", len(h.Held))
	}
}

// Two page-tier members make a page-tier message that pages once.
func TestOutagePagesOnce(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	e, seen := outageEngine(t, now)
	in := outageDecisions("shop", 5, now)
	in[1].Incident.Tier, in[3].Incident.Tier = incident.Page, incident.Page

	e.announcer.collect.CollectOutages(context.Background(), now, in)

	paging := 0
	for _, m := range *seen {
		if m.PagingOnly {
			paging++
		}
	}
	if paging != 1 || (*seen)[0].Status != notification.StatusCritical {
		t.Fatalf("paging = %d, outage = %+v", paging, (*seen)[0])
	}
}

// The outage closes with the roll-up resolve once all members resolved.
func TestOutageClosesWithTheRollupResolve(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	e, seen := outageEngine(t, now)
	e.announcer.collect.CollectOutages(context.Background(), now,
		outageDecisions("shop", 5, now))
	e.announcer.collect.Startup.CheckSummary = true

	sent := e.announcer.collect.CloseListings(context.Background())

	last := (*seen)[len(*seen)-1]
	if !sent || last.Status != notification.StatusResolved ||
		!strings.HasPrefix(last.Key, notification.RollupKeyPrefix) {
		t.Fatalf("sent=%v last=%+v", sent, last)
	}
}

// A held incident that was paged and then resolves before the message
// goes still gets its paging resolve, so its alert closes. One that was
// never paged is left out silently.
func TestOutageResolveOfAPagedMemberReachesPaging(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	e, seen := outageEngine(t, now)
	h := e.announcer.collect.Outages["shop"]
	h.Held = outageDecisions("shop", 3, now)
	quiet := outageDecision("shop", 1, now)
	quiet.Action = incident.Resolve
	paged := outageDecision("shop", 2, now)
	paged.Action = incident.Resolve
	paged.Incident.Delivery.MarkPaged()

	e.announcer.collect.HoldOutage(context.Background(), now, quiet)
	if got := pagingOnly(*seen); got != 0 {
		t.Fatalf("an unpaged resolve sent %d paging messages", got)
	}
	e.announcer.collect.HoldOutage(context.Background(), now, paged)

	if got := pagingOnly(*seen); got != 1 {
		t.Fatalf("want one paging-only resolve, got %d", got)
	}
}

func pagingOnly(messages []notification.Message) int {
	n := 0
	for _, m := range messages {
		if m.PagingOnly {
			n++
		}
	}
	return n
}
