package dashboard

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// sessionServer is a test server whose clock the test moves.
type sessionServer struct {
	*Server
	now time.Time
}

func newSessionServer(shared bool, password string) *sessionServer {
	s := &sessionServer{now: testEpoch}
	s.Server = NewServer(Options{Token: testToken, Password: password,
		Shared: shared, TrustProxy: shared, Edition: "Pro"},
		clock.Func(func() time.Time { return s.now }))
	s.SetIncidents(&fakeIncidents{})
	return s
}

func (s *sessionServer) do(req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func loginRequest(password string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// signIn signs in and returns the session cookie.
func (s *sessionServer) signIn(t *testing.T, password string) *http.Cookie {
	t.Helper()
	w := s.do(loginRequest(password))
	if w.Code != http.StatusNoContent {
		t.Fatalf("login: status %d %s", w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatal("login set no session cookie")
	return nil
}

func (s *sessionServer) getWith(path string, c *http.Cookie) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if c != nil {
		req.AddCookie(c)
	}
	return s.do(req).Code
}

func TestSessionCookieOpensTheAPIAndHasSafeAttributes(t *testing.T) {
	s := newSessionServer(false, testPassword)
	c := s.signIn(t, testPassword)

	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode ||
		c.Path != "/" || c.Domain != "" || c.Secure {
		t.Errorf("local cookie = %+v", c)
	}
	if c.MaxAge != int((12 * time.Hour).Seconds()) {
		t.Errorf("max age = %d, want 12h", c.MaxAge)
	}
	for _, path := range []string{"/api/session", "/api/incidents"} {
		if code := s.getWith(path, c); code/100 != 2 {
			t.Errorf("%s with cookie: %d", path, code)
		}
	}
	if code := s.getWith("/api/session", nil); code != 401 {
		t.Errorf("/api/session without cookie: %d, want 401", code)
	}
}

func TestSessionPasswordAcceptsTheGroupedFormInAnyCase(t *testing.T) {
	s := newSessionServer(false, testPassword)
	for _, typed := range []string{"7k4mqx9p2hvdt6re",
		"7K4M QX9P 2HVD T6RE", testPassword} {
		s.signIn(t, typed)
	}
	if w := s.do(loginRequest("7K4M-QX9P-2HVD-T6RF")); w.Code != 401 {
		t.Errorf("wrong password: %d, want 401", w.Code)
	}
	if w := s.do(loginRequest("")); w.Code != 401 {
		t.Errorf("empty password: %d, want 401", w.Code)
	}
}

func TestSessionEndsAfterTwelveHoursAndOnPasswordReset(t *testing.T) {
	s := newSessionServer(false, testPassword)
	c := s.signIn(t, testPassword)

	s.now = testEpoch.Add(12*time.Hour - time.Second)
	if code := s.getWith("/api/session", c); code != 204 {
		t.Errorf("before 12h: %d, want 204", code)
	}
	s.now = testEpoch.Add(12 * time.Hour)
	if code := s.getWith("/api/session", c); code != 401 {
		t.Errorf("at 12h: %d, want 401", code)
	}

	s.now = testEpoch
	reset := newSessionServer(false, "NEWP-ASSW-ORD0-0000")
	if code := reset.getWith("/api/session", c); code != 401 {
		t.Errorf("after reset: %d, want 401", code)
	}
	again := newSessionServer(false, testPassword)
	if code := again.getWith("/api/session", c); code != 204 {
		t.Errorf("restart, same password: %d, want 204", code)
	}
}

func TestSessionLogoutRevokesTheCookie(t *testing.T) {
	s := newSessionServer(false, testPassword)
	c := s.signIn(t, testPassword)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(c)
	w := s.do(req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", w.Code)
	}
	cleared := w.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Errorf("logout cookies = %+v", cleared)
	}
	if code := s.getWith("/api/session", c); code != 401 {
		t.Errorf("revoked cookie: %d, want 401", code)
	}
}

func TestSessionTamperedCookieIsRefused(t *testing.T) {
	s := newSessionServer(false, testPassword)
	c := s.signIn(t, testPassword)
	forged := *c
	i := strings.Index(c.Value, ".") + 5
	flip := byte('A')
	if c.Value[i] == 'A' {
		flip = 'B'
	}
	forged.Value = c.Value[:i] + string(flip) + c.Value[i+1:]
	if code := s.getWith("/api/incidents", &forged); code != 401 {
		t.Errorf("forged cookie: %d, want 401", code)
	}
	noPassword := newSessionServer(false, "")
	if w := noPassword.do(loginRequest("")); w.Code != 401 {
		t.Errorf("no password configured: %d, want 401", w.Code)
	}
}

func TestSessionSharedModeNeedsHTTPS(t *testing.T) {
	s := newSessionServer(true, testPassword)

	plain := loginRequest(testPassword)
	plain.RemoteAddr = "10.0.0.5:1234"
	if w := s.do(plain); w.Code != 403 ||
		!strings.Contains(w.Body.String(), "https_required") {
		t.Errorf("plain http: %d %s", w.Code, w.Body.String())
	}
	portForward := loginRequest(testPassword)
	portForward.RemoteAddr = "127.0.0.1:1234"
	portForward.Header.Set("X-Forwarded-Proto", "https")
	if w := s.do(portForward); w.Code != 403 {
		t.Errorf("loopback claiming https: %d, want 403", w.Code)
	}
	session := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	session.RemoteAddr = "10.0.0.5:1234"
	if w := s.do(session); w.Code != 403 {
		t.Errorf("session check over http: %d, want 403", w.Code)
	}

	viaIngress := loginRequest(testPassword)
	viaIngress.RemoteAddr = "10.0.0.5:1234"
	viaIngress.Header.Set("X-Forwarded-Proto", "https")
	w := s.do(viaIngress)
	if w.Code != 204 {
		t.Fatalf("https via ingress: %d", w.Code)
	}
	if c := w.Result().Cookies()[0]; !c.Secure {
		t.Errorf("shared cookie is not Secure: %+v", c)
	}
}

func TestSessionLoginRefusesCrossSiteAndNonJSON(t *testing.T) {
	s := newSessionServer(false, testPassword)
	cross := loginRequest(testPassword)
	cross.Header.Set("Origin", "https://evil.example")
	if w := s.do(cross); w.Code != 403 {
		t.Errorf("cross-site: %d, want 403", w.Code)
	}
	form := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader("password="+testPassword))
	form.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if w := s.do(form); w.Code != 400 {
		t.Errorf("form post: %d, want 400", w.Code)
	}
	get := httptest.NewRequest(http.MethodGet, "/login", nil)
	if w := s.do(get); w.Code != 405 {
		t.Errorf("GET /login: %d, want 405", w.Code)
	}
	same := loginRequest(testPassword)
	same.Header.Set("Origin", "http://"+same.Host)
	if w := s.do(same); w.Code != 204 {
		t.Errorf("same origin: %d, want 204", w.Code)
	}
}

func TestSessionLoginFailuresAreRateLimited(t *testing.T) {
	s := newSessionServer(false, testPassword)
	for range 10 {
		if w := s.do(loginRequest("wrong")); w.Code != 401 {
			t.Fatalf("wrong password: %d", w.Code)
		}
	}
	w := s.do(loginRequest(testPassword))
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Errorf("after 10 failures: %d retry %q", w.Code,
			w.Header().Get("Retry-After"))
	}
	s.now = testEpoch.Add(61 * time.Second)
	s.signIn(t, testPassword)
}

func TestSessionSharedModeLimitsTheClientBehindTheIngress(t *testing.T) {
	s := newSessionServer(true, testPassword)
	attempt := func(client, password string) int {
		req := loginRequest(password)
		req.RemoteAddr = "10.0.0.5:1234"
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-For", client)
		return s.do(req).Code
	}
	for range 10 {
		attempt("203.0.113.7", "wrong")
	}
	if code := attempt("203.0.113.7", testPassword); code != 429 {
		t.Errorf("guessing client: %d, want 429", code)
	}
	if code := attempt("198.51.100.2", testPassword); code != 204 {
		t.Errorf("other client behind the same ingress: %d, want 204", code)
	}
}

func TestSessionMissingCredentialsDoNotCountAsFailures(t *testing.T) {
	s := newSessionServer(false, testPassword)
	for range 20 {
		s.getWith("/api/session", nil)
	}
	s.signIn(t, testPassword)
}

func TestSessionForwardedProtoNeedsTrustProxy(t *testing.T) {
	s := newSessionServer(true, testPassword)
	s.Server = NewServer(Options{Token: testToken, Password: testPassword,
		Shared: true, Edition: "Pro"},
		clock.Func(func() time.Time { return s.now }))
	claimed := loginRequest(testPassword)
	claimed.RemoteAddr = "10.0.0.5:1234"
	claimed.Header.Set("X-Forwarded-Proto", "https")
	if w := s.do(claimed); w.Code != 403 {
		t.Errorf("untrusted X-Forwarded-Proto: %d, want 403", w.Code)
	}
	direct := loginRequest(testPassword)
	direct.TLS = &tls.ConnectionState{}
	if w := s.do(direct); w.Code != 204 {
		t.Errorf("kwatch's own TLS: %d, want 204", w.Code)
	}
}
