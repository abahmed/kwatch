package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func cookieRequest(value string) *http.Request {
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: value})
	return req
}

// After a restart every open tab presents a session that is no longer
// valid on each poll. Those are not guesses: they must not use up the
// address's failure limit and lock out the next sign-in.
func TestAuthStaleSessionsDoNotCountAsGuesses(t *testing.T) {
	auth := NewAuth("secret-token", "pw", false, clock.RealClock{})
	handler := auth.Middleware(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {}))
	for i := 0; i < 30; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, cookieRequest("stale-session"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: status = %d, want 401", i, w.Code)
		}
	}
	if !auth.allowed(cookieRequest("")) {
		t.Fatal("stale sessions used up the failure limit")
	}
}

// Guesses from a shared address lock out further guesses, not the
// people already signed in from it.
func TestAuthLimitedAddressKeepsValidSessions(t *testing.T) {
	auth := NewAuth("secret-token", "pw", false, clock.RealClock{})
	handler := auth.Middleware(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {}))
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("GET", "/api/incidents", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	session, _, err := auth.sessions.issue()
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, cookieRequest(session))

	if w.Code != http.StatusOK {
		t.Errorf("signed-in session on a limited address: status = %d",
			w.Code)
	}
}

// Search looks at the cause as the API shows it, redacted, so a search
// cannot tell what a redacted secret contains.
func TestSearchDoesNotMatchRedactedText(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.Cause = &rootcause.CauseRecord{
		Summary: "login fails: password=hunter2 rejected"}
	s := newTestServer(p)

	var list ListResponse
	body := get(t, s, "/api/incidents?state=active&q=hunter2").Body.Bytes()
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 0 {
		t.Errorf("search matched redacted text: %+v", list.Incidents)
	}
}

// A list row carries a short cause, so a long one cannot push incidents
// out of the response; the detail carries it whole.
func TestListCauseIsShortAndDetailCauseWhole(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.Cause = &rootcause.CauseRecord{Summary: strings.Repeat("x", 500)}
	s := newTestServer(p)

	var list ListResponse
	if err := json.Unmarshal(
		get(t, s, "/api/incidents?state=active").Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var d IncidentDetail
	if err := json.Unmarshal(
		get(t, s, "/api/incidents/a").Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(list.Incidents[0].RootCause)); n != maxListCause {
		t.Errorf("list cause has %d characters, want %d", n, maxListCause)
	}
	if len(d.RootCause) != 500 {
		t.Errorf("detail cause has %d characters, want 500",
			len(d.RootCause))
	}
}
