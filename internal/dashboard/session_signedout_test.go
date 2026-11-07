package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
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

func TestSessionRevocationIsNeverForgottenWhileFull(t *testing.T) {
	now := testEpoch
	s := newSessions("pw", "tok", clock.Func(func() time.Time { return now }))
	var all []string
	for range maxRevoked {
		v, _, _ := s.issue()
		all = append(all, v)
		s.revoke(v)
	}
	survivor, _, _ := s.issue()
	last, _, _ := s.issue()
	s.revoke(last)
	for _, v := range all {
		if s.valid(v) {
			t.Fatal("a revoked session became valid again")
		}
	}
	if s.valid(last) || s.valid(survivor) {
		t.Error("a full table must sign everyone out, not forget revoked")
	}
	fresh, _, _ := s.issue()
	if !s.valid(fresh) {
		t.Error("a new sign-in after the rotation does not work")
	}
}

func TestSessionRevocationsExpireWithTheirSession(t *testing.T) {
	now := testEpoch
	s := newSessions("pw", "tok", clock.Func(func() time.Time { return now }))
	v, _, _ := s.issue()
	s.revoke(v)
	now = now.Add(sessionLifetime + time.Minute)
	other, _, _ := s.issue()
	s.revoke(other)
	if len(s.revoked) != 1 {
		t.Errorf("revoked = %d, want only the live one", len(s.revoked))
	}
}

func TestSessionFromEarlierStartIsRecognized(t *testing.T) {
	clk := clock.Func(func() time.Time { return testEpoch })
	old := newSessions("pw", "tok", clk)
	v, _, _ := old.issue()
	restarted := newSessions("pw", "tok", clk)
	if restarted.valid(v) || !restarted.fromEarlierStart(v) {
		t.Error("a cookie from before a restart was not recognized")
	}
	if restarted.fromEarlierStart("junk") {
		t.Error("junk is not a cookie from an earlier start")
	}
	fresh, _, _ := restarted.issue()
	if restarted.fromEarlierStart(fresh) {
		t.Error("a current cookie was called stale")
	}
}

func TestSessionCheckSaysTheSignOutWasARestart(t *testing.T) {
	old := newSessionServer(false, testPassword)
	c := old.signIn(t, testPassword)
	s := newSessionServer(false, testPassword)
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(c)
	w := s.do(req)
	if w.Code != 200 || !strings.Contains(w.Body.String(),
		`"reason":"restarted"`) {
		t.Errorf("stale cookie: %d %q", w.Code, w.Body.String())
	}
	plain := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	if w := s.do(plain); strings.Contains(w.Body.String(), "reason") {
		t.Errorf("a visitor with no cookie got a reason: %q",
			w.Body.String())
	}
}
