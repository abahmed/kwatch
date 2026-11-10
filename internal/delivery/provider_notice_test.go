package delivery

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

type noticeRig struct {
	m       *Manager
	clk     *fakeClock
	entries map[string]*providerEntry
	mu      sync.Mutex
	sent    []string
}

func newNoticeRig(t *testing.T, names ...string) *noticeRig {
	t.Helper()
	r := &noticeRig{clk: newFakeClock(), entries: map[string]*providerEntry{}}
	var list []providerEntry
	for _, n := range names {
		p := newMessageProvider(func(int, notification.Message) error {
			return nil
		})
		list = append(list, providerEntry{catalogName: n, provider: p})
	}
	r.m = managerWithEntries(list)
	r.m.now = r.clk.Now
	r.m.sendNotice = func(target, text string) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.sent = append(r.sent, target+": "+text)
	}
	for _, n := range names {
		e := r.m.generation.entries[n]
		r.entries[n] = &e
		r.m.setProviderHealth(&e, "", nil)
	}
	return r
}

func (r *noticeRig) fail(name, reason string) {
	r.m.setProviderHealth(r.entries[name], reason, errors.New("boom"))
}

func (r *noticeRig) ok(name string) {
	r.m.setProviderHealth(r.entries[name], "", nil)
}

func (r *noticeRig) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.sent
	r.sent = nil
	return out
}

func TestNoNoticeInsideTheGrace(t *testing.T) {
	r := newNoticeRig(t, "slack", "email")
	r.fail("slack", reasonRejected)
	r.clk.Advance(noticeGrace - time.Second)
	r.m.CheckNotices()
	assert.Empty(t, r.take())
}

func TestFailingProviderIsToldToTheWorkingOnesOnce(t *testing.T) {
	r := newNoticeRig(t, "slack", "email")
	r.fail("slack", reasonRejected)
	r.clk.Advance(noticeGrace)
	r.m.CheckNotices()
	got := r.take()
	require.Len(t, got, 1)
	assert.True(t, strings.HasPrefix(got[0], "email: kwatch can't send "+
		"to slack since 09:00 UTC"), got[0])
	assert.Contains(t, got[0], "webhook, token or address may have expired")
	assert.Contains(t, got[0], "installer, under Notifications")

	r.clk.Advance(time.Hour)
	r.m.CheckNotices()
	r.m.CheckNotices()
	assert.Empty(t, r.take(), "no repeats while nothing changed")
}

func TestRecoveryIsToldOnce(t *testing.T) {
	r := newNoticeRig(t, "slack", "email")
	r.fail("slack", reasonUnavailable)
	r.clk.Advance(noticeGrace)
	r.m.CheckNotices()
	r.take()

	r.ok("slack")
	r.m.CheckNotices()
	got := r.take()
	require.Len(t, got, 2, "both working providers hear it")
	assert.Contains(t, got[0]+got[1], "kwatch is sending to slack again")
	r.m.CheckNotices()
	assert.Empty(t, r.take())
}

func TestAShortBlipIsNeverToldOrRecovered(t *testing.T) {
	r := newNoticeRig(t, "slack", "email")
	r.fail("slack", reasonUnavailable)
	r.clk.Advance(30 * time.Second)
	r.ok("slack")
	r.clk.Advance(time.Hour)
	r.m.CheckNotices()
	assert.Empty(t, r.take())
}

func TestWhenEveryProviderFailsItIsLoggedAndNothingIsSent(t *testing.T) {
	r := newNoticeRig(t, "slack", "email")
	r.fail("slack", reasonUnavailable)
	r.fail("email", reasonRejected)
	r.clk.Advance(noticeGrace)
	r.m.CheckNotices()
	assert.Empty(t, r.take(), "no provider works to carry a notice")
	assert.True(t, r.m.allFailing)
	r.m.CheckNotices()
	r.ok("email")
	r.m.CheckNotices()
	assert.False(t, r.m.allFailing)
	got := r.take()
	require.Len(t, got, 1, "email then hears slack is down")
	assert.Contains(t, got[0], "can't send to slack")
}

// Only a refusal is fixed in the installer: an outage or a rate limit
// says kwatch keeps trying. The recovery does not claim every waiting
// alert went out: some may have aged out meanwhile.
func TestNoticeWordingFitsTheReason(t *testing.T) {
	since := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	for _, reason := range []string{reasonUnavailable, reasonRateLimited} {
		text := failureNoticeText("slack", reason, since, 3)
		assert.NotContains(t, text, "Fix:", reason)
		assert.Contains(t, text, "keeps trying", reason)
	}
	assert.Contains(t, failureNoticeText("slack", reasonRejected, since, 3),
		"Fix: update slack in the installer")
	recovered := recoveryNoticeText("slack", 3)
	assert.NotContains(t, recovered, "went out")
	assert.Contains(t, recovered, "waiting alerts are being sent")
}
