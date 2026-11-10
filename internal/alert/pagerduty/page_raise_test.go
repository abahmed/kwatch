package pagerduty

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// events sends m and returns "action key severity" for each event.
func events(t *testing.T, m notification.Message) []string {
	t.Helper()
	c, rec := newTestPagerDuty(t)
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	var out []string
	for _, r := range rec.Requests() {
		var p pagerdutyPayload
		if err := json.Unmarshal(r.Body, &p); err != nil {
			t.Fatal(err)
		}
		line := p.EventAction + " " + p.DedupKey
		if p.Payload != nil {
			line += " " + p.Payload.Severity
		}
		out = append(out, line)
	}
	return out
}

func sameEvents(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events =\n%s\nwant\n%s", strings.Join(got, "\n"),
			strings.Join(want, "\n"))
	}
}

// The update that raises a page heard at warning opens the page key at
// critical first, then closes the warning alert: the incident it opened
// keeps its low urgency, so only a new alert pages.
func TestPagerDutyRaisedPageOpensPageKeyThenClosesBase(t *testing.T) {
	m := providertest.Update()
	m.Route.Severity = "warning"
	m.RaisesPage, m.PageKeyed = true, true
	sameEvents(t, events(t, m),
		"trigger kwatch-dev-p-42-page critical",
		"resolve kwatch-dev-p-42")
}

// Every later message goes to the page key alone, the lowering update
// too.
func TestPagerDutyPageKeyedUpdateUsesPageKey(t *testing.T) {
	m := providertest.Update()
	m.Route.Severity = "warning"
	m.PagingOnly, m.PageKeyed = true, true
	sameEvents(t, events(t, m), "trigger kwatch-dev-p-42-page warning")
}

// A keyed resolve closes both keys; an unkeyed one closes one.
func TestPagerDutyResolveClosesPageKeyToo(t *testing.T) {
	m := providertest.Resolve()
	m.PageKeyed = true
	sameEvents(t, events(t, m),
		"resolve kwatch-dev-p-42", "resolve kwatch-dev-p-42-page")
	sameEvents(t, events(t, providertest.Resolve()),
		"resolve kwatch-dev-p-42")
}

// A key too long for the Events API keeps its "-page" ending under the
// cap and stays apart from the capped base key.
func TestPagerDutyCappedPageKeyKeepsSuffix(t *testing.T) {
	m := providertest.Resolve()
	m.Key, m.PageKeyed = strings.Repeat("k", 400), true
	got := events(t, m)
	base := strings.TrimPrefix(got[0], "resolve ")
	paged := strings.TrimPrefix(got[1], "resolve ")
	if len(paged) > maxDedupKey || len(base) > maxDedupKey ||
		!strings.HasSuffix(paged, "-page") || paged == base {
		t.Fatalf("base %q (%d), page %q (%d)", base, len(base), paged,
			len(paged))
	}
}
