package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestSelfReportReadsTheSessionAndTheStateFile(t *testing.T) {
	now := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	state := openTestStore(t)
	require.NoError(t, newOutboxStore(state).WriteOutbox(
		[]delivery.OutboxRecord{{ID: "a"}, {ID: "b"}}, nil))
	stop := health.SelfStop{At: now.Add(-time.Hour), Reason: "oom_killed"}
	p := &pipelineBuild{deps: &serverDeps{}, state: state,
		clock: clock.Func(func() time.Time { return now }),
		gap: startNotice{self: selfSession{
			startedAt: now.Add(-time.Hour), lastStop: &stop,
			restarts: []health.SelfStop{stop,
				{At: now.Add(-25 * time.Hour), Reason: "eviction"}},
		}},
		links: &dashboardLinks{target: testDashboard,
			model: inventory.NewModel(inventory.Options{})},
	}

	got := p.selfReport()

	assert.Equal(t, now.Add(-time.Hour), got.StartedAt)
	assert.Equal(t, &stop, got.LastStop)
	assert.Equal(t, []health.SelfStop{stop}, got.Restarts,
		"only the last day")
	assert.Equal(t, 2, *got.OutboxPending)
	assert.Positive(t, got.StateFileBytes)
	assert.Equal(t, health.LinkStatus{Reason: health.LinksNoRoute},
		got.DashboardLinks)
	assert.False(t, got.PodLogsForbidden)
	assert.Nil(t, got.Startup, "no source yet")
}

func TestSelfHolderReportsOnlyWhileASessionRuns(t *testing.T) {
	var h selfHolder
	_, ok := h.Report()
	require.False(t, ok)

	h.set(func() health.SelfReport {
		return health.SelfReport{OutboxPending: intp(3)}
	})
	got, ok := h.Report()
	require.True(t, ok)
	assert.Equal(t, 3, *got.OutboxPending)

	h.set(nil)
	_, ok = h.Report()
	assert.False(t, ok)
}

func intp(n int) *int { return &n }
