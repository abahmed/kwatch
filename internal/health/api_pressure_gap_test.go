package health

import (
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/status"
)

// API pressure reads as reduced coverage on /status and as a degraded
// component on /health while it lasts.
func TestStatusNamesAPIPressureAsReducedCoverage(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.SetComponentStatus(APIPressureComponent, "degraded", "rate_limited",
		false)

	text := getStatus(h, http.MethodGet, "/status").Body.String()
	if !strings.Contains(text,
		"reduced coverage: the API server is throttling kwatch") {
		t.Fatalf("no API pressure gap in\n%s", text)
	}
	if got := h.ComponentErrors()[APIPressureComponent]; got !=
		"rate_limited" {
		t.Fatalf("component reason = %q", got)
	}

	h.SetComponentStatus(APIPressureComponent, "running", "", true)
	text = getStatus(h, http.MethodGet, "/status").Body.String()
	if strings.Contains(text, "throttling kwatch") {
		t.Fatalf("gap left after the pressure ended:\n%s", text)
	}
}
