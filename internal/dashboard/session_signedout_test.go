package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionSignedOutAnswers200SignedInFalseOnly(t *testing.T) {
	s := newSessionServer(false, testPassword)
	c := s.signIn(t, testPassword)
	revoked := httptest.NewRequest(http.MethodPost, "/logout", nil)
	revoked.AddCookie(c)
	s.do(revoked)

	for name, cookie := range map[string]*http.Cookie{"none": nil,
		"revoked": c, "garbage": {Name: sessionCookie, Value: "x"}} {
		req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := s.do(req)
		if w.Code != 200 || w.Body.String() != "{\"signedIn\":false}\n" {
			t.Errorf("%s: %d %q", name, w.Code, w.Body.String())
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q", name, got)
		}
		if w.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%s: Content-Type = %q", name,
				w.Header().Get("Content-Type"))
		}
	}
	in := s.signIn(t, testPassword)
	if code := s.getWith("/api/session", in); code != 204 {
		t.Errorf("signed in: %d, want 204", code)
	}
}

func TestSessionSignedOutChecksDoNotCountAsFailures(t *testing.T) {
	s := newSessionServer(false, testPassword)
	for range 50 {
		w := s.do(httptest.NewRequest(http.MethodGet, "/api/session", nil))
		if w.Code != 200 {
			t.Fatalf("signed-out check: %d", w.Code)
		}
	}
	s.signIn(t, testPassword)
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	if w := s.do(req); w.Code != 204 {
		t.Errorf("right token after 50 checks: %d, want 204", w.Code)
	}
}

func TestSessionWrongBearerTokenStillGets401AndCounts(t *testing.T) {
	s := newSessionServer(false, testPassword)
	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		if w := s.do(req); w.Code != 401 {
			t.Fatalf("wrong token: %d, want 401", w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if w := s.do(req); w.Code != 429 {
		t.Errorf("11th guess: %d, want 429", w.Code)
	}
}

func TestSessionSignedOutKeepsEveryOtherRoute401(t *testing.T) {
	s := newSessionServer(false, testPassword)
	for _, path := range []string{"/api/incidents", "/api/status",
		"/api/overview", "/api/events", "/api/activity", "/api/nothing"} {
		if code := s.getWith(path, nil); code != 401 {
			t.Errorf("%s signed out: %d, want 401", path, code)
		}
	}
}
