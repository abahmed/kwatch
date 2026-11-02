package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/status"
)

// The dashboard reads kwatch's self report and failed sends from the
// deps it was built with, once a session publishes the report.
func TestServerDepsWireTheDashboardSelfReport(t *testing.T) {
	now := clock.Func(func() time.Time { return time.Unix(100, 0) })
	dash := dashboard.NewServer(dashboard.Options{Token: "tok"}, now)
	dash.SetStatus(func() status.Report { return status.Report{} })
	deps := newServerDeps(t.Context(), func() {}, &bootstrap{
		entitlements: feature.ProEntitlements(),
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, now),
		dashboardServer: dash, deliveryManager: &delivery.Manager{},
		clock: now,
	})
	read := func() dashboard.StatusResponse {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		req.Header.Set("Authorization", "Bearer tok")
		w := httptest.NewRecorder()
		dash.Handler().ServeHTTP(w, req)
		var out dashboard.StatusResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		return out
	}
	if got := read(); got.Self != nil || got.FailedLastHour == nil ||
		*got.FailedLastHour != 0 {
		t.Fatalf("before a session: %+v", got)
	}
	deps.self.set(func() health.SelfReport {
		return health.SelfReport{OutboxPending: 4}
	})
	if got := read(); got.Self == nil || got.Self.OutboxPending != 4 {
		t.Fatalf("with a session: %+v", got.Self)
	}
}

func TestDashboardDeliveryDetailWithNothingSentIsEmpty(t *testing.T) {
	got := dashboardDeliveryDetail(&delivery.Manager{})("inc-1")
	if len(got.Pager) != 0 || len(got.Sent) != 0 {
		t.Errorf("detail = %+v", got)
	}
}
