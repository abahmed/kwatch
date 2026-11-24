package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// sessionGet calls path with the session cookie of a sign-in.
func sessionGet(h http.Handler, cookie *http.Cookie) int {
	req := httptest.NewRequest(http.MethodGet, "/api/incidents", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

func sessionCookieOf(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatalf("sign-in set no session cookie (status %d)", w.Code)
	return nil
}

// A configured lifetime is when a sign-in ends, and what the cookie
// tells the browser.
func TestSessionExpiresAtTheConfiguredLifetime(t *testing.T) {
	now := testEpoch
	s := NewServer(Options{Token: testToken, Password: testPassword,
		SessionLifetime: 72 * time.Hour},
		clock.Func(func() time.Time { return now }))
	s.SetIncidents(&fakeIncidents{})
	h := s.Handler()
	cookie := sessionCookieOf(t, signIn(h, testPassword))
	if cookie.MaxAge != int((72 * time.Hour).Seconds()) {
		t.Errorf("cookie MaxAge = %d, want 72h", cookie.MaxAge)
	}
	now = testEpoch.Add(sessionLifetime + time.Hour)
	if got := sessionGet(h, cookie); got != http.StatusOK {
		t.Fatalf("after 13h: status %d, want the session alive", got)
	}
	now = testEpoch.Add(72*time.Hour - time.Second)
	if got := sessionGet(h, cookie); got != http.StatusOK {
		t.Fatalf("just before 72h: status %d, want 200", got)
	}
	now = testEpoch.Add(72 * time.Hour)
	if got := sessionGet(h, cookie); got != http.StatusUnauthorized {
		t.Fatalf("at 72h: status %d, want 401", got)
	}
}

// Without a lifetime the default holds.
func TestSessionLifetimeDefaultsToTwelveHours(t *testing.T) {
	now := testEpoch
	s := NewServer(Options{Token: testToken, Password: testPassword},
		clock.Func(func() time.Time { return now }))
	s.SetIncidents(&fakeIncidents{})
	h := s.Handler()
	cookie := sessionCookieOf(t, signIn(h, testPassword))
	now = testEpoch.Add(sessionLifetime)
	if got := sessionGet(h, cookie); got != http.StatusUnauthorized {
		t.Fatalf("at 12h: status %d, want 401", got)
	}
}

// The sign-in help is shown as text above the docs link: markup in it
// is escaped, and without it the page has no trace of it.
func TestSignInHelpIsEscapedText(t *testing.T) {
	s := NewServer(Options{SignInHelp: "Ask #ops <b>now</b>\n" +
		"or see the wiki"}, clock.RealClock{})
	body := shellBody(t, s)
	want := `<p class="help" data-signin-help>Ask #ops ` +
		`&lt;b&gt;now&lt;/b&gt;<br>or see the wiki</p>`
	if !strings.Contains(body, want) {
		t.Fatalf("shell lacks the escaped help %q", want)
	}
	if strings.Index(body, want) > strings.Index(body,
		"How to get the password") {
		t.Error("the help must come before the docs link")
	}
	plain := shellBody(t, NewServer(Options{}, clock.RealClock{}))
	if strings.Contains(plain, "data-signin-help") ||
		strings.Contains(plain, signInHelpMark) {
		t.Error("without help text the page must show none")
	}
}

func shellBody(t *testing.T, s *Server) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/signin", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w.Body.String()
}
