package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/status"
)

func TestStatusTextNeedsProWhenDenied(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.DenyStatusText()

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := getStatus(h, method, "/status")
		if w.Code != http.StatusPaymentRequired {
			t.Fatalf("%s code = %d, want 402", method, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(
			ct, "text/plain") {
			t.Fatalf("%s content type %q", method, ct)
		}
	}
	w := getStatus(h, http.MethodGet, "/status")
	want := "The /status page is part of kwatch Pro. " +
		"/health stays available."
	if got := strings.TrimSpace(w.Body.String()); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestStatusJSONKeepsItsRefusalWhenTextIsDenied(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.DenyStatusJSON()
	h.DenyStatusText()

	w := getStatus(h, http.MethodGet, "/status?format=json")
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("code = %d, want 402", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got !=
		`{"error":"pro_required"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestStatusTextReopensWhenAllowed(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.DenyStatusText()
	h.AllowStatusText()
	if w := getStatus(h, http.MethodGet, "/status"); w.Code != 200 {
		t.Fatalf("code = %d after the edition became Pro", w.Code)
	}
}

func TestStatusDeniedLeavesHealthServed(t *testing.T) {
	h := NewHealthServerWithClock(
		config.HealthCheck{StatusEndpoint: true}, clock.RealClock{})
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.DenyStatusText()
	h.DenyStatusJSON()
	for path, want := range map[string]int{
		"/health": http.StatusOK, "/healthz": http.StatusOK,
		"/status": http.StatusPaymentRequired,
	} {
		w := httptest.NewRecorder()
		newServeMux(h).ServeHTTP(w,
			httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != want {
			t.Errorf("%s = %d, want %d", path, w.Code, want)
		}
	}
}
