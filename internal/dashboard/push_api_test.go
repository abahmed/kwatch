package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// pushCall sends body as JSON to path with the test token.
func pushCall(
	t *testing.T, s *Server, method, path, body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func subBody(endpoint, min string) string {
	b, _ := json.Marshal(pushRequest{Subscription: testSub(endpoint),
		Min: min})
	return string(b)
}

func TestPushAPIWithoutThePipelineAnswers503(t *testing.T) {
	s := newTestServer()
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/push", ""},
		{http.MethodPost, "/api/push", subBody(chrome, "page")},
		{http.MethodPost, "/api/push/remove", `{"endpoint":"x"}`},
		{http.MethodPost, "/api/push/status", `{"endpoint":"x"}`},
		{http.MethodPost, "/api/push/test", `{"endpoint":"x"}`},
	} {
		if w := pushCall(t, s, c.method, c.path, c.body, nil); w.Code != 503 {
			t.Errorf("%s %s: %d", c.method, c.path, w.Code)
		}
	}
}

func TestPushAPISubscribesTestsAndRemoves(t *testing.T) {
	s := newTestServer()
	p, f := newTestPusher(t, openPushStore(t))
	s.sources.push.Store(p)
	w := pushCall(t, s, http.MethodGet, "/api/push", "", nil)
	var key pushKey
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &key) != nil ||
		key.PublicKey != p.book.key.Public() {
		t.Fatalf("key: %d %s", w.Code, w.Body)
	}
	status := func() pushStatus {
		t.Helper()
		w := pushCall(t, s, http.MethodPost, "/api/push/status",
			`{"endpoint":"`+chrome+`"}`, nil)
		var st pushStatus
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &st) != nil {
			t.Fatalf("status: %d %s", w.Code, w.Body)
		}
		return st
	}
	if status().Subscribed {
		t.Fatal("subscribed before subscribing")
	}
	if w := pushCall(t, s, http.MethodPost, "/api/push",
		subBody(chrome, "notify"), nil); w.Code != 204 {
		t.Fatalf("subscribe: %d %s", w.Code, w.Body)
	}
	if !status().Subscribed {
		t.Fatal("not subscribed after subscribing")
	}
	if w := pushCall(t, s, http.MethodPost, "/api/push/test",
		`{"endpoint":"`+chrome+`"}`, nil); w.Code != 204 || f.to(chrome) != 1 {
		t.Fatalf("test: %d, %d sent", w.Code, f.to(chrome))
	}
	f.fail[chrome] = &transport.StatusError{StatusCode: 403}
	w = pushCall(t, s, http.MethodPost, "/api/push/test",
		`{"endpoint":"`+chrome+`"}`, nil)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "signature") {
		t.Errorf("failed test: %d %s", w.Code, w.Body)
	}
	if w := pushCall(t, s, http.MethodPost, "/api/push/test",
		`{"endpoint":"`+firefox+`"}`, nil); w.Code != 404 {
		t.Errorf("test to an unknown browser: %d", w.Code)
	}
	for i := 0; i < 2; i++ {
		if w := pushCall(t, s, http.MethodPost, "/api/push/remove",
			`{"endpoint":"`+chrome+`"}`, nil); w.Code != 204 {
			t.Fatalf("remove %d: %d", i, w.Code)
		}
	}
	if on, _ := p.book.status(chrome); on {
		t.Error("removed browser still subscribed")
	}
}

func TestPushAPIRefusesBadRequests(t *testing.T) {
	s := newTestServer()
	p, _ := newTestPusher(t, openPushStore(t))
	s.sources.push.Store(p)
	for name, c := range map[string]struct {
		body    string
		headers map[string]string
		code    int
	}{
		"unknown host": {subBody("https://evil.example/x", "page"), nil, 400},
		"metadata":     {subBody("https://169.254.169.254/x", "page"), nil, 400},
		"bad level":    {subBody(chrome, "silent"), nil, 400},
		"bad keys": {`{"subscription":{"endpoint":"` + chrome +
			`","keys":{"p256dh":"AAAA","auth":"AAAA"}},"min":"page"}`,
			nil, 400},
		"not JSON": {subBody(chrome, "page"),
			map[string]string{"Content-Type": "text/plain"}, 400},
		"other site": {subBody(chrome, "page"),
			map[string]string{"Origin": "https://evil.example"}, 403},
		"too big": {`{"min":"page","x":"` + strings.Repeat("a", maxPushBody) +
			`"}`, nil, 400},
	} {
		w := pushCall(t, s, http.MethodPost, "/api/push", c.body, c.headers)
		if w.Code != c.code {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if subs := p.book.targets(testEpoch); len(subs) != 0 {
		t.Errorf("%d bad subscriptions kept", len(subs))
	}
	for _, path := range []string{"/api/push/remove", "/api/push/test",
		"/api/push/status"} {
		if w := pushCall(t, s, http.MethodGet, path, "", nil); w.Code != 405 {
			t.Errorf("GET %s: %d", path, w.Code)
		}
	}
	if w := pushCall(t, s, http.MethodDelete, "/api/push", "",
		nil); w.Code != 405 {
		t.Errorf("DELETE /api/push: %d", w.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/push",
		strings.NewReader(subBody(chrome, "page")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
}

func TestServiceWorkerIsServedAtTheRoot(t *testing.T) {
	s := newTestServer()
	w := call(t, s, http.MethodGet, "/sw.js", "")
	if w.Code != 200 ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") ||
		w.Header().Get("Cache-Control") != "no-cache" ||
		!strings.Contains(w.Body.String(), "requireInteraction: true") {
		t.Errorf("/sw.js: %d %v", w.Code, w.Header())
	}
	// The notification icon is named by its hashed address, which
	// serves a PNG.
	icon := regexp.MustCompile(`/assets/icon-192\.[0-9a-f]{12}\.png`).
		FindString(w.Body.String())
	if icon == "" || strings.Contains(w.Body.String(), "{{asset:") {
		t.Fatalf("/sw.js names no hashed icon")
	}
	if iw := call(t, s, http.MethodGet, icon, ""); iw.Code != 200 ||
		iw.Header().Get("Content-Type") != "image/png" {
		t.Errorf("%s: %d %v", icon, iw.Code, iw.Header())
	}
}

// A browser that renews its subscription (pushsubscriptionchange)
// keeps its settings under the new address and the old one goes.
func TestPushAPIRenewalKeepsTheSettings(t *testing.T) {
	s := newTestServer()
	p, _ := newTestPusher(t, openPushStore(t))
	s.sources.push.Store(p)
	_ = p.book.subscribe(testSub(chrome), pushPrefs{Min: "page",
		Resolved: true}, "", testEpoch)
	b, _ := json.Marshal(pushRequest{Subscription: testSub(chrome2),
		Replaces: chrome})
	if w := pushCall(t, s, http.MethodPost, "/api/push", string(b),
		nil); w.Code != 204 {
		t.Fatalf("renew: %d %s", w.Code, w.Body)
	}
	if on, _ := p.book.status(chrome); on {
		t.Error("the old address is kept")
	}
	if got, ok := p.book.prefsOf(chrome2); !ok ||
		got != (pushPrefs{Min: "page", Resolved: true}) {
		t.Errorf("new settings = %+v, %v", got, ok)
	}
	b, _ = json.Marshal(pushRequest{Subscription: testSub(firefox),
		Replaces: "https://fcm.googleapis.com/fcm/send/unknown"})
	if w := pushCall(t, s, http.MethodPost, "/api/push", string(b),
		nil); w.Code != 400 {
		t.Errorf("renewing an unknown browser without settings: %d", w.Code)
	}
}

// The app manifest is served at the root, its icons by their hashed
// address, so the dashboard can be installed as an app.
func TestAppManifestIsServedAtTheRoot(t *testing.T) {
	s := newTestServer()
	w := call(t, s, http.MethodGet, "/manifest.webmanifest", "")
	if w.Code != 200 || w.Header().Get("Content-Type") !=
		"application/manifest+json" ||
		w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("manifest: %d %v", w.Code, w.Header())
	}
	var m struct {
		Name     string
		StartURL string `json:"start_url"`
		Display  string
		Icons    []struct{ Src, Sizes string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "kwatch" || m.StartURL != "/" ||
		m.Display != "standalone" || len(m.Icons) != 2 {
		t.Fatalf("manifest: %+v", m)
	}
	for _, i := range m.Icons {
		iw := call(t, s, http.MethodGet, i.Src, "")
		if iw.Code != 200 || iw.Header().Get("Content-Type") != "image/png" {
			t.Errorf("icon %s (%s): %d", i.Src, i.Sizes, iw.Code)
		}
	}
}
