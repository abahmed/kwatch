package app

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
)

// An edition rebuild keeps the health listener but stops the dashboard:
// the next generation builds its own, and the old one must not keep
// serving the old edition or hold the port.
func TestEditionRebuildStopsTheDashboard(t *testing.T) {
	for _, rebuild := range []bool{true, false} {
		now := clock.Func(func() time.Time { return time.Unix(100, 0) })
		dash := dashboard.NewServer(dashboard.Options{Token: "tok"}, now)
		if err := dash.Open(t.Context()); err != nil {
			t.Fatal(err)
		}
		deps := newServerDeps(t.Context(), func() {}, &bootstrap{
			entitlements: feature.ProEntitlements(),
			healthServer: health.NewHealthServerWithClock(
				config.HealthCheck{}, now),
			dashboardServer: dash, deliveryManager: &delivery.Manager{},
			clock: now,
		})
		stopServersAtExit(deps, rebuild)
		if err := dash.Open(t.Context()); err == nil {
			t.Errorf("rebuild=%v: the dashboard still runs", rebuild)
		}
	}
}
