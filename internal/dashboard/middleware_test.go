package dashboard

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
	"testing"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/status"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})
	return &out
}

func TestSecurityHeadersAreOnEveryResponse(t *testing.T) {
	s := newTestServer()
	unauth := call(t, s, http.MethodGet, "/api/status", "")
	missing := get(t, s, "/nope")
	for range 10 {
		call(t, s, http.MethodGet, "/api/status", "wrong")
	}
	limited := call(t, s, http.MethodGet, "/api/status", "wrong")

	for name, w := range map[string]*http.Response{
		"401": unauth.Result(), "404": missing.Result(),
		"429": limited.Result(), "200": get(t, s, "/api/status").Result(),
	} {
		h := w.Header
		if h.Get("X-Content-Type-Options") != "nosniff" ||
			h.Get("X-Frame-Options") != "DENY" ||
			h.Get("Cache-Control") != "no-store" ||
			h.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s (status %d) headers = %v", name, w.StatusCode, h)
		}
	}
	if limited.Code != http.StatusTooManyRequests {
		t.Errorf("limited status = %d", limited.Code)
	}
}

// A failed request is logged at the default level with host, path and
// status; a successful read (a page polling) is not, so it cannot drown
// the rest of the log. The token is never logged.
func TestAccessLogNamesHostPathAndStatusButNotTheToken(t *testing.T) {
	out := captureLog(t)
	s := newTestServer()

	get(t, s, "/api/status")
	call(t, s, http.MethodGet, "/api/status", "wrong-token")
	klog.Flush()

	logged := out.String()
	if strings.Contains(logged, "status=200") {
		t.Errorf("a successful read is logged at the default level: %s",
			logged)
	}
	for _, want := range []string{`"dashboard request"`,
		`path="/api/status"`, "status=401", `remote="192.0.2.1"`} {
		if !strings.Contains(logged, want) {
			t.Errorf("log missing %s: %s", want, logged)
		}
	}
	if strings.Contains(logged, testToken) ||
		strings.Contains(logged, "Authorization") {
		t.Errorf("log carries the token: %s", logged)
	}
}

func TestSourcesCanChangeWhileRequestsAreServed(t *testing.T) {
	s := newTestServer(testIncident("a", "shop", 0))
	reader := &fakeIncidents{items: nil}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200 {
			s.SetIncidents(nil)
			s.SetStatus(nil)
			s.SetDeliveryHealth(nil)
			s.SetIncidents(reader)
			s.SetStatus(func() status.Report { return status.Report{} })
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			for _, path := range []string{"/api/incidents",
				"/api/incidents/a", "/api/status"} {
				code := get(t, s, path).Code
				if code >= 500 && code != http.StatusServiceUnavailable {
					t.Errorf("%s: status %d", path, code)
				}
			}
		}
	}()
	wg.Wait()
}
