package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/status"
)

type noticeLog struct{ msgs []string }

func (n *noticeLog) send(m string) { n.msgs = append(n.msgs, m) }

// noticeSession is a license session over a shared state file, as a new
// process generation would create it.
func noticeSession(
	d diskState, log *noticeLog, now time.Time,
) *licenseSession {
	return &licenseSession{
		disk: d, send: log.send, now: func() time.Time { return now },
	}
}

func proStatus(state license.State) license.Status {
	return license.Status{
		State: state, Plan: license.PlanPro, LID: "lic_…1234",
		ExpiresAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestLicenseNoticesOncePerTransitionAcrossRestarts(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var log noticeLog

	active := proStatus(license.StateActive)
	noticeSession(disk, &log, now).notify(active)
	require.Len(t, log.msgs, 1)
	require.Contains(t, log.msgs[0], "kwatch Pro is enabled")

	// A restart and a hundred ticks repeat nothing.
	for range 100 {
		noticeSession(disk, &log, now).notify(active)
	}
	require.Len(t, log.msgs, 1)

	grace := proStatus(license.StateGrace)
	grace.GraceUntil = now.Add(5 * 24 * time.Hour)
	for range 3 {
		noticeSession(disk, &log, now).notify(grace)
	}
	require.Len(t, log.msgs, 2)
	require.Contains(t, log.msgs[1], "grace period")

	expired := license.Status{
		State: license.StateExpired, Reason: "license expired",
	}
	for range 3 {
		noticeSession(disk, &log, now).notify(expired)
	}
	require.Len(t, log.msgs, 3)
	require.Contains(t, log.msgs[2], "kwatch Pro is disabled")
	require.Contains(t, log.msgs[2], "license expired")

	// Renewing again is a new transition and is announced.
	noticeSession(disk, &log, now).notify(active)
	require.Len(t, log.msgs, 4)
	require.Contains(t, log.msgs[3], "kwatch Pro is enabled")
}

func TestLicenseNoticesCoverEveryReasonThatDisablesPro(t *testing.T) {
	for _, tc := range []struct {
		state  license.State
		reason string
	}{
		{license.StateExpired, "license expired"},
		{license.StateRevoked, "license revoked"},
		{license.StateConflict, "license active on another cluster"},
		{license.StateInvalid, "bad signature"},
	} {
		t.Run(string(tc.state), func(t *testing.T) {
			rec := licenseNotified{Pro: true}
			msgs, rec := licenseNotices(rec, license.Status{
				State: tc.state, Reason: tc.reason,
			}, time.Time{})
			require.Len(t, msgs, 1)
			require.Contains(t, msgs[0], tc.reason)
			require.False(t, rec.Pro)
		})
	}
}

func TestLicenseNoticesIgnorePendingAndCommunityNeverEnabled(t *testing.T) {
	msgs, _ := licenseNotices(licenseNotified{Pro: true},
		license.Status{State: license.StatePending}, time.Time{})
	require.Empty(t, msgs, "pending is not a loss of Pro")
	msgs, _ = licenseNotices(licenseNotified{},
		license.Status{State: license.StateCommunity}, time.Time{})
	require.Empty(t, msgs)
}

func TestLicenseTrialWarningsFireOnceEach(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	exp := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	trial := license.Status{
		State: license.StateActive, Plan: license.PlanTrial,
		LID: "lic_…9999", ExpiresAt: exp,
	}
	var log noticeLog
	at := func(daysLeft int) time.Time {
		return exp.Add(-time.Duration(daysLeft) * 24 * time.Hour)
	}
	noticeSession(disk, &log, at(20)).notify(trial)
	require.Len(t, log.msgs, 1, "only Pro enabled so far")

	for range 3 {
		noticeSession(disk, &log, at(14)).notify(trial)
	}
	require.Len(t, log.msgs, 2)
	require.Contains(t, log.msgs[1], "license ends in 14 day")

	for range 3 {
		noticeSession(disk, &log, at(9)).notify(trial)
	}
	require.Len(t, log.msgs, 2, "nothing between the thresholds")

	for range 3 {
		noticeSession(disk, &log, at(3)).notify(trial)
	}
	require.Len(t, log.msgs, 3)
	require.Contains(t, log.msgs[2], "license ends in 3 day")
}

func TestLicenseNoticeIsNotSentWhenItCannotBeRecorded(t *testing.T) {
	store := openTestStore(t)
	disk := diskState{store: store}
	require.NoError(t, store.Close())
	var log noticeLog
	noticeSession(disk, &log, time.Now()).notify(
		proStatus(license.StateActive))
	require.Empty(t, log.msgs, "an unrecorded notice would repeat")
}

func TestLicenseViewReportsTheSectionWithoutTheKey(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	deps := &serverDeps{}
	deps.edition.Store(feature.CommunityEntitlements())
	l := &licenseSession{
		deps: deps, now: func() time.Time { return now },
		tracker: &renewTracker{now: func() time.Time { return now }},
	}
	l.tracker.record("success")
	l.tracker.scheduled(24 * time.Hour)
	st := license.Status{
		State: license.StateConflict, Plan: license.PlanPro,
		LID: "lic_…abcd", Reason: "license active on another cluster",
		ExpiresAt: now.Add(10*24*time.Hour + time.Hour),
	}
	l.cur.Store(&st)

	view := l.view()

	require.Equal(t, "Community", view.Edition)
	require.Equal(t, "conflict", view.State)
	require.Equal(t, "lic_…abcd", view.LID)
	require.Equal(t, 10, *view.DaysLeft)
	require.Equal(t, now, *view.LastRenew)
	require.Equal(t, now.Add(24*time.Hour), *view.NextAttempt)
	require.Equal(t, status.ConflictHint, view.Hint)
	require.Contains(t, view.Hint, "https://kwatch.dev/account")
	require.Contains(t, view.Hint, "bound to another cluster")
	require.Nil(t, (&licenseSession{
		deps: deps, tracker: l.tracker, now: l.now,
	}).view())
}

func TestEditionHolderDefaultsToCommunity(t *testing.T) {
	var h editionHolder
	require.Equal(t, feature.CommunityEdition, h.Load().Edition())
	h.Store(feature.ProEntitlements())
	require.Equal(t, feature.ProEdition, h.Load().Edition())
	h.Store(nil)
	require.Equal(t, feature.CommunityEdition, h.Load().Edition())
}

func TestEntitlementsForFollowsProEnabled(t *testing.T) {
	for state, want := range map[license.State]feature.Edition{
		license.StateActive:    feature.ProEdition,
		license.StateGrace:     feature.ProEdition,
		license.StateCommunity: feature.CommunityEdition,
		license.StatePending:   feature.CommunityEdition,
		license.StateInvalid:   feature.CommunityEdition,
		license.StateExpired:   feature.CommunityEdition,
		license.StateRevoked:   feature.CommunityEdition,
		license.StateConflict:  feature.CommunityEdition,
	} {
		got := entitlementsFor(license.Status{State: state}).Edition()
		require.Equal(t, want, got, string(state))
	}
}

func TestLicenseEndpointDefaultMatchesTheClient(t *testing.T) {
	require.Equal(t, license.DefaultEndpoint, config.DefaultLicenseEndpoint)
	require.Equal(t, license.DefaultEndpoint,
		config.DefaultConfig().License.Endpoint)
}
