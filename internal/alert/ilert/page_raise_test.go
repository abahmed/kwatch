package ilert

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// sent sends m and returns "event key priority" for each request.
func sent(
	t *testing.T, rec *providertest.Recorder, c *Ilert,
	m notification.Message,
) ([]string, error) {
	t.Helper()
	rec.Reset()
	err := c.SendIncident(context.Background(), m)
	var out []string
	for _, r := range rec.Requests() {
		p := decodePayload(t, r.Body)
		out = append(out, strings.TrimSpace(
			p.EventType+" "+p.AlertKey+" "+p.Priority))
	}
	return out, err
}

func wantSent(t *testing.T, got []string, err error, want ...string) {
	t.Helper()
	if err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("sent =\n%s\nwant\n%s", strings.Join(got, "\n"),
			strings.Join(want, "\n"))
	}
}

// The raise of a page heard at LOW opens the page key at the paging
// priority first, then closes the LOW alert, which never escalates.
func TestIlertRaisedPageOpensPageKeyThenClosesBase(t *testing.T) {
	c, rec := newTestIlert(t)
	m := providertest.Update()
	m.Route.Severity = "warning"
	m.RaisesPage, m.PageKeyed = true, true
	got, err := sent(t, rec, c, m)
	wantSent(t, got, err,
		"ALERT kwatch-dev-p-42-page HIGH", "RESOLVE kwatch-dev-p-42")

	m.RaisesPage = false
	got, err = sent(t, rec, c, m)
	wantSent(t, got, err, "ALERT kwatch-dev-p-42-page LOW")
}

// A keyed resolve closes both keys; an unkeyed one closes one.
func TestIlertResolveClosesPageKeyToo(t *testing.T) {
	c, rec := newTestIlert(t)
	m := providertest.Resolve()
	m.PageKeyed = true
	got, err := sent(t, rec, c, m)
	wantSent(t, got, err,
		"RESOLVE kwatch-dev-p-42 HIGH", "RESOLVE kwatch-dev-p-42-page HIGH")
	got, err = sent(t, rec, c, providertest.Resolve())
	wantSent(t, got, err, "RESOLVE kwatch-dev-p-42 HIGH")
}

// A pager that never heard the incident below the page tier has no alert
// under the base key: a refused resolve of it is no failure, and the
// page key is still closed. The same answer on the page key is one.
func TestIlertBaseResolveOfUnknownKeyIsTolerated(t *testing.T) {
	c, rec := newTestIlert(t)
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		if !strings.Contains(string(r.Body), "-page") {
			w.WriteHeader(http.StatusNotFound)
		}
	}
	m := providertest.Resolve()
	m.PageKeyed = true
	got, err := sent(t, rec, c, m)
	wantSent(t, got, err,
		"RESOLVE kwatch-dev-p-42 HIGH", "RESOLVE kwatch-dev-p-42-page HIGH")

	m = providertest.Update()
	m.RaisesPage, m.PageKeyed = true, true
	if _, err := sent(t, rec, c, m); err != nil {
		t.Fatalf("raise with an unknown base key: %v", err)
	}

	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusNotFound)
	}
	if _, err := sent(t, rec, c, m); err == nil {
		t.Fatal("a refused page key must fail the message")
	}
}
