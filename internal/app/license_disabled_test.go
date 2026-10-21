package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/license"
)

func invalidStatus(reason string) license.Status {
	return license.Status{State: license.StateInvalid, Reason: reason}
}

// A key that was mistyped, expired or revoked before Pro ever ran must
// still say so: Community is not what the operator configured.
func TestLicenseNoticeTellsAKeyThatNeverRanPro(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var log noticeLog
	bad := invalidStatus("license: malformed: bad payload encoding")

	for range 3 {
		noticeSession(disk, &log, now).notify(bad)
	}

	require.Len(t, log.msgs, 1, "once across restarts")
	require.Contains(t, log.msgs[0], "kwatch Pro is disabled")
	require.Contains(t, log.msgs[0], "bad payload encoding")
	require.Contains(t, log.msgs[0], "Community edition")
}

func TestLicenseNoticeRepeatsOnlyForADistinctReason(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var log noticeLog

	noticeSession(disk, &log, now).notify(invalidStatus("reason one"))
	noticeSession(disk, &log, now).notify(invalidStatus("reason one"))
	noticeSession(disk, &log, now).notify(invalidStatus("reason two"))
	require.Len(t, log.msgs, 2)

	// A working license, then the same lapse again, is a new event.
	noticeSession(disk, &log, now).notify(proStatus(license.StateActive))
	noticeSession(disk, &log, now).notify(invalidStatus("reason two"))
	require.Len(t, log.msgs, 4, "enabled, then disabled again")
}

func TestLicenseNoticeStaysQuietWithoutAKeyOrWhilePending(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var log noticeLog

	noticeSession(disk, &log, now).notify(
		license.Status{State: license.StateCommunity})
	noticeSession(disk, &log, now).notify(
		license.Status{State: license.StatePending, Reason: "awaiting"})

	require.Empty(t, log.msgs)
}

func licenseHealthSession(
	t *testing.T, key string,
) (*licenseSession, *health.HealthServer) {
	t.Helper()
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})
	deps := &serverDeps{healthServer: server}
	if key != "" {
		deps.license = &licenseSetup{key: key}
	}
	return &licenseSession{deps: deps}, server
}

func TestLicenseHealthComponentShowsAKeyThatCannotRunPro(t *testing.T) {
	l, server := licenseHealthSession(t, "KW1-bad")

	l.reportHealth(invalidStatus("license: malformed"))

	got := server.ComponentStatuses()[licenseComponent]
	require.Equal(t, "degraded", got.State)
	require.Equal(t, "license_invalid", got.Reason)
	require.False(t, got.Available)
	require.Equal(t, "license_invalid",
		server.ComponentErrors()[licenseComponent],
		"a bad key makes /health report degraded")

	l.reportHealth(proStatus(license.StateActive))
	got = server.ComponentStatuses()[licenseComponent]
	require.Equal(t, "running", got.State)
	require.Empty(t, server.ComponentErrors())
}

func TestLicenseHealthComponentWaitsBeforeTheAnswer(t *testing.T) {
	l, server := licenseHealthSession(t, "KW1-x")

	l.reportHealth(license.Status{State: license.StatePending})

	got := server.ComponentStatuses()[licenseComponent]
	require.Equal(t, "waiting", got.State)
	require.Equal(t, "license_pending", got.Reason)
	require.Empty(t, server.ComponentErrors())
}

func TestLicenseHealthComponentIsAbsentWithoutAKey(t *testing.T) {
	l, server := licenseHealthSession(t, "")

	l.reportHealth(license.Status{State: license.StateCommunity})

	_, shown := server.ComponentStatuses()[licenseComponent]
	require.False(t, shown, "Community by choice is not a degradation")
}
