package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
)

func bearerStatus(h http.Handler, token string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

func signIn(h http.Handler, password string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// A server built without credentials refuses everything; credentials
// that arrive later work on the running server.
func TestSetCredentialsUnlocksARunningServer(t *testing.T) {
	s := NewServer(Options{}, clock.RealClock{})
	h := s.Handler()
	if s.HasCredentials() {
		t.Fatal("a server without credentials reports some")
	}
	if got := bearerStatus(h, "tok"); got != http.StatusUnauthorized {
		t.Fatalf("before: token status = %d, want 401", got)
	}
	if got := signIn(h, "ABCD-EFGH-JKLM-NPQR").Code; got !=
		http.StatusUnauthorized {
		t.Fatalf("before: sign-in status = %d, want 401", got)
	}

	s.SetCredentials("tok", "ABCD-EFGH-JKLM-NPQR")

	if !s.HasCredentials() {
		t.Fatal("credentials were not kept")
	}
	if got := bearerStatus(h, "tok"); got != http.StatusNoContent {
		t.Fatalf("after: token status = %d, want 204", got)
	}
	if got := bearerStatus(h, "other"); got != http.StatusUnauthorized {
		t.Fatalf("after: wrong token status = %d, want 401", got)
	}
	// The generated grouped form is accepted without dashes, lower-case.
	w := signIn(h, "abcdefghjklmnpqr")
	if w.Code != http.StatusNoContent {
		t.Fatalf("after: sign-in status = %d, want 204", w.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent ||
		strings.Contains(rec.Body.String(), "signedIn") {
		t.Fatalf("session after sign-in: %d %q", rec.Code,
			rec.Body.String())
	}
}

// New credentials sign out the sessions of the old ones.
func TestSetCredentialsSignsOldSessionsOut(t *testing.T) {
	s := NewServer(Options{Token: "tok", Password: "old"},
		clock.RealClock{})
	value, _, err := s.auth.sessions.issue()
	if err != nil {
		t.Fatal(err)
	}
	if !s.auth.sessions.valid(value) {
		t.Fatal("a fresh session is not valid")
	}

	s.SetCredentials("tok", "new")

	if s.auth.sessions.valid(value) {
		t.Fatal("a session of the old password is still valid")
	}
}
