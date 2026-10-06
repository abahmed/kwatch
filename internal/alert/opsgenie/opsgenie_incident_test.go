package opsgenie

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

func newTestOpsgenie(t *testing.T) (*Opsgenie, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewOpsgenie(map[string]interface{}{"apiKey": "secret"}, "dev",
		rec.Dependencies())
	c.url = rec.URL() + "/v2/alerts"
	return c, rec
}

func TestOpsgenieIncidentLifecycle(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			rec.Reset()
			if err := c.SendIncident(context.Background(), m); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			req := rec.Requests()[0]
			if req.Header.Get("Authorization") != "GenieKey secret" {
				t.Fatal("missing GenieKey authorization")
			}
			if m.Resolved() {
				want := "/v2/alerts/" + m.AlertKey("dev") + "/close"
				if req.Path != want || req.Query != "identifierType=alias" {
					t.Fatalf("resolve went to %s?%s", req.Path, req.Query)
				}
				return
			}
			var p ogPayload
			if err := json.Unmarshal(req.Body, &p); err != nil {
				t.Fatal(err)
			}
			if p.Alias != m.AlertKey("dev") || p.Alias != "kwatch-dev-p-42" {
				t.Fatalf("alias = %q", p.Alias)
			}
			if p.Message != m.Short || p.Priority != "P1" {
				t.Fatalf("payload = %+v", p)
			}
			providertest.AssertOneLeadingEmoji(t, p.Message)
			if !strings.Contains(p.Description, m.Note) ||
				p.Details["Cluster"] != "dev" {
				t.Fatalf("payload = %+v", p)
			}
		})
	}
}

func TestOpsgenieMessageIsTruncated(t *testing.T) {
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 300)
	p := (&Opsgenie{}).buildPayload(m)
	if len(p.Message) > messageLimit || !strings.HasSuffix(p.Message, "…") {
		t.Fatalf("message = %d bytes", len(p.Message))
	}
}

func TestOpsgenieEscapesAliasWithSlash(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	m := providertest.Update()
	m.Key = "pod/shop/web"
	m.Route.Severity = "critical"
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	m.Revision, m.Status = 3, notification.StatusResolved
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	escaped := "/v2/alerts/kwatch-dev-pod%2Fshop%2Fweb/"
	actions := 0
	for _, req := range rec.Requests() {
		if req.Path == "/v2/alerts" {
			continue
		}
		actions++
		if !strings.HasPrefix(req.RawPath, escaped) {
			t.Fatalf("alias not escaped: %s", req.RawPath)
		}
	}
	if actions != 4 {
		t.Fatalf("alias actions = %d, want 3 updates and 1 close", actions)
	}
}

func TestOpsgenieSkipsNotices(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	providertest.AssertNoticesSkipped(t, c, rec)
}

func TestOpsgenieUpdateEscalatesOpenAlert(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	m := providertest.Update()
	m.Route.Severity = "critical"
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	requests := rec.Requests()
	alertPath := "/v2/alerts/kwatch-dev-p-42/"
	want := []struct{ method, path, field, value string }{
		{"POST", "/v2/alerts", "alias", "kwatch-dev-p-42"},
		{"PUT", alertPath + "priority", "priority", "P1"},
		{"PUT", alertPath + "message", "message", m.Short},
		{"PUT", alertPath + "description", "description", ""},
	}
	if len(requests) != len(want) {
		t.Fatalf("requests = %d, want %d", len(requests), len(want))
	}
	for i, w := range want {
		req := requests[i]
		if req.Method != w.method || req.Path != w.path {
			t.Fatalf("request %d = %s %s, want %s %s",
				i, req.Method, req.Path, w.method, w.path)
		}
		if i > 0 && req.Query != "identifierType=alias" {
			t.Fatalf("request %d query = %q", i, req.Query)
		}
		got, _ := req.JSON(t)[w.field].(string)
		if w.value != "" && got != w.value {
			t.Fatalf("request %d %s = %q, want %q", i, w.field, got, w.value)
		}
		if w.field == "description" && !strings.Contains(got, m.Note) {
			t.Fatalf("description = %q", got)
		}
	}
}

func TestOpsgenieFirstRevisionOnlyCreates(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	err := c.SendIncident(context.Background(), providertest.Announce())
	if err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	if got := len(rec.Requests()); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestOpsgenieUpdateFailureIsReturned(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	err := c.SendIncident(context.Background(), providertest.Update())
	if err == nil || !strings.Contains(err.Error(), "priority") {
		t.Fatalf("err = %v", err)
	}
}

// Closing an alias Opsgenie does not know (404) means the alert is already
// gone, so the resolve succeeds instead of failing for good.
func TestOpsgenieCloseOfUnknownAliasSucceeds(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusNotFound)
	}
	if err := c.SendIncident(
		context.Background(), providertest.Resolve()); err != nil {
		t.Fatalf("close of a missing alert must succeed: %v", err)
	}
}

func TestOpsgenieCloseOtherFailureStillFails(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	if err := c.SendIncident(
		context.Background(), providertest.Resolve()); err == nil {
		t.Fatal("a 403 on close must still fail")
	}
}
