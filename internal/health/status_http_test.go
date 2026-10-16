package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/status"
)

func newStatusServer() *HealthServer {
	return NewHealthServerWithClock(config.HealthCheck{}, clock.RealClock{})
}

func getStatus(
	h *HealthServer, method, target string,
) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.statusHandler(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestStatusIsUnavailableWithoutAPipeline(t *testing.T) {
	h := newStatusServer()
	if w := getStatus(h, http.MethodGet, "/status"); w.Code != 503 {
		t.Fatalf("code = %d", w.Code)
	}
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.SetStatusSource(nil)
	if w := getStatus(h, http.MethodGet, "/status"); w.Code != 503 {
		t.Fatalf("code after nil = %d", w.Code)
	}
}

func TestStatusIsReadOnly(t *testing.T) {
	h := newStatusServer()
	if w := getStatus(h, http.MethodPost, "/status"); w.Code != 405 {
		t.Fatalf("POST code = %d", w.Code)
	}
}

func TestStatusTextAndJSONAddCoverageGaps(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.SetCoverage(CoverageSummary{Complete: true, Unavailable: 1,
		Reasons: map[string]int{CoveragePermissionDenied: 1},
		UnavailableKinds: []CoverageKind{{Kind: "secrets",
			Reason: CoveragePermissionDenied}}})
	h.SetComponentStatus("kubelet-stats", "degraded", "unreachable", false)

	text := getStatus(h, http.MethodGet, "/status")
	if ct := text.Header().Get("Content-Type"); !strings.HasPrefix(
		ct, "text/plain") {
		t.Fatalf("content type %q", ct)
	}
	for _, want := range []string{
		"secrets objects: forbidden, kwatch cannot read it",
		"node stats from the kubelets: unreachable or degraded"} {
		if !strings.Contains(text.Body.String(), want) {
			t.Errorf("missing %q in\n%s", want, text.Body)
		}
	}
	js := getStatus(h, http.MethodGet, "/status?format=json")
	if ct := js.Header().Get("Content-Type"); ct != "application/json" ||
		!strings.Contains(js.Body.String(), `"coverageGaps"`) {
		t.Fatalf("json response: %q %s", ct, js.Body)
	}
}

func TestStatusIsNotFoundByDefault(t *testing.T) {
	h := NewHealthServerWithClock(
		config.HealthCheck{Enabled: true, Port: 8060}, clock.RealClock{})
	h.SetStatusSource(func() status.Report { return status.Report{} })
	w := httptest.NewRecorder()
	newServeMux(h).ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	if w.Code != 404 {
		t.Fatalf("code = %d, want 404 while statusEndpoint is off", w.Code)
	}
}

func TestStatusIsServedOnTheMux(t *testing.T) {
	h := NewHealthServerWithClock(
		config.HealthCheck{StatusEndpoint: true}, clock.RealClock{})
	h.SetStatusSource(func() status.Report { return status.Report{} })
	w := httptest.NewRecorder()
	newServeMux(h).ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	if w.Code != 200 {
		t.Fatalf("code = %d", w.Code)
	}
}

func TestStatusOutputIsBounded(t *testing.T) {
	report := status.Report{}
	for i := 0; i < 5000; i++ {
		report.Upgrade.Items = append(report.Upgrade.Items,
			status.Blocker{Text: strings.Repeat("y", 200)})
	}
	body, _ := renderStatus(report, "")
	if len(body) > maxStatusBytes+100 {
		t.Fatalf("body of %d bytes", len(body))
	}
	body, _ = renderStatus(report, "json")
	if len(body) > maxStatusBytes {
		t.Fatalf("json of %d bytes", len(body))
	}
}

func TestStatusSourceCanChangeWhileServing(t *testing.T) {
	h := newStatusServer()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			h.SetStatusSource(func() status.Report { return status.Report{} })
			h.SetCoverage(CoverageSummary{Complete: true})
		}()
		go func() {
			defer wg.Done()
			getStatus(h, http.MethodGet, "/status")
		}()
	}
	wg.Wait()
}

func TestStatusJSONNeedsProWhenDenied(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.DenyStatusJSON()

	w := getStatus(h, http.MethodGet, "/status?format=json")
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("code = %d, want 402", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got !=
		`{"error":"pro_required"}` {
		t.Fatalf("body = %q", got)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type %q", ct)
	}
}

func TestStatusTextIsUnchangedWhenJSONIsDenied(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.DenyStatusJSON()
	if w := getStatus(h, http.MethodGet, "/status"); w.Code != 200 {
		t.Fatalf("text code = %d", w.Code)
	}
}

func TestStatusJSONIsServedByDefault(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	if w := getStatus(h, http.MethodGet, "/status?format=json"); w.Code !=
		200 {
		t.Fatalf("code = %d", w.Code)
	}
}

func TestStatusServesTheLicenseSectionInTextAndJSON(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.SetLicenseSource(func() *status.License {
		return &status.License{
			Edition: "Pro", State: "active", LID: "lic_…abcd",
		}
	})

	text := getStatus(h, http.MethodGet, "/status").Body.String()
	if !strings.Contains(text, "License ID: lic_…abcd") {
		t.Fatalf("text has no license section\n%s", text)
	}
	body := getStatus(h, http.MethodGet, "/status?format=json").Body.String()
	if !strings.Contains(body, `"license"`) ||
		!strings.Contains(body, `"state": "active"`) {
		t.Fatalf("json has no license object\n%s", body)
	}

	h.SetLicenseSource(nil)
	body = getStatus(h, http.MethodGet, "/status?format=json").Body.String()
	if strings.Contains(body, `"license"`) {
		t.Fatalf("a cleared source must leave the license out\n%s", body)
	}
}

func TestStatusJSONStaysDeniedOnCommunityAndReopensOnPro(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.SetLicenseSource(func() *status.License {
		return &status.License{Edition: "Community", State: "expired"}
	})
	h.DenyStatusJSON()

	w := getStatus(h, http.MethodGet, "/status?format=json")
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("code = %d, want 402", w.Code)
	}
	if strings.Contains(w.Body.String(), "license") {
		t.Fatalf("a 402 must not carry the license object: %s", w.Body)
	}
	text := getStatus(h, http.MethodGet, "/status").Body.String()
	if !strings.Contains(text, "State: expired") {
		t.Fatalf("Community text keeps the license section\n%s", text)
	}

	h.AllowStatusJSON()
	if w := getStatus(h, http.MethodGet, "/status?format=json"); w.Code !=
		http.StatusOK {
		t.Fatalf("code = %d after the edition became Pro", w.Code)
	}
}
