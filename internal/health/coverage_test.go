package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func newCoverageServer() *HealthServer {
	return NewHealthServerWithClock(config.HealthCheck{}, clock.RealClock{})
}

func TestCoverageIsAbsentBeforePublication(t *testing.T) {
	if got := newCoverageServer().Coverage(); got != nil {
		t.Fatalf("coverage = %+v, want nil", got)
	}
}

func TestCoverageBoundsKindsAndReasons(t *testing.T) {
	server := newCoverageServer()
	var kinds []CoverageKind
	for i := 0; i < MaxCoverageKinds+10; i++ {
		kinds = append(kinds, CoverageKind{
			Kind: fmt.Sprintf("k%03d.example", i), Reason: "raw: secret",
		})
	}
	server.SetCoverage(CoverageSummary{
		Watched: map[string]int{"full": 3}, Unavailable: len(kinds),
		UnavailableKinds: kinds, Complete: true,
	})

	got := server.Coverage()
	if len(got.UnavailableKinds) != MaxCoverageKinds || !got.Truncated {
		t.Fatalf("kinds = %d truncated = %v", len(got.UnavailableKinds),
			got.Truncated)
	}
	if got.Unavailable != MaxCoverageKinds+10 {
		t.Fatalf("total unavailable = %d", got.Unavailable)
	}
	for _, kind := range got.UnavailableKinds {
		if kind.Reason != CoverageAPIUnavailable {
			t.Fatalf("reason %q not bounded", kind.Reason)
		}
	}
	got.Watched["full"] = 99
	if server.Coverage().Watched["full"] != 3 {
		t.Fatal("coverage is not detached")
	}
}

func TestHealthEndpointServesCoverageWithoutAffectingReadiness(t *testing.T) {
	server := newCoverageServer()
	server.SetCoverage(CoverageSummary{
		Skipped: 2, Unavailable: 1, Complete: false,
		UnavailableKinds: []CoverageKind{{
			Kind:   "leases.coordination.k8s.io",
			Reason: CoveragePermissionDenied,
		}},
	})
	recorder := httptest.NewRecorder()
	server.healthHandler(recorder,
		httptest.NewRequest(http.MethodGet, "/health", nil))

	var body HealthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Coverage == nil || body.Coverage.Skipped != 2 ||
		body.Coverage.UnavailableKinds[0].Reason != "permission_denied" {
		t.Fatalf("coverage = %+v", body.Coverage)
	}
	if body.Status != "ok" || server.Ready() {
		t.Fatalf("coverage changed status %q or readiness", body.Status)
	}
}

func TestCoverageReasonCountsStayInVocabulary(t *testing.T) {
	server := newCoverageServer()
	server.SetCoverage(CoverageSummary{Reasons: map[string]int{
		CoverageObjectCapReached: 2, CoverageBudgetExceeded: 3,
		"raw: secret": 1, CoverageAPIUnavailable: 1,
	}})

	got := server.Coverage().Reasons
	want := map[string]int{
		CoverageObjectCapReached: 2, CoverageBudgetExceeded: 3,
		CoverageAPIUnavailable: 2,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("reasons = %v, want %v", got, want)
	}
	got[CoverageBudgetExceeded] = 99
	if server.Coverage().Reasons[CoverageBudgetExceeded] != 3 {
		t.Fatal("reasons are not detached")
	}
}
