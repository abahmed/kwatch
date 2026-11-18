package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

type sentSelf struct{ msgs []notification.Message }

func (s *sentSelf) NotifyIncident(m notification.Message) {
	s.msgs = append(s.msgs, m)
}

type memSelfStore struct{ state selfAlertState }

func (m *memSelfStore) loadSelfAlerts() (selfAlertState, error) {
	return m.state, nil
}

func (m *memSelfStore) saveSelfAlerts(s selfAlertState) error {
	m.state = s
	return nil
}

// selfHarness is an alerter over components a test sets.
type selfHarness struct {
	parts map[string]health.ComponentStatus
	sent  *sentSelf
	store *memSelfStore
	a     *selfAlerter
	// proof is whether a read of a proof part has succeeded.
	proof bool
}

func newSelfHarness(store *memSelfStore, updates bool) *selfHarness {
	h := &selfHarness{parts: map[string]health.ComponentStatus{},
		sent: &sentSelf{}, store: store}
	probes := &selfProbes{components: func() map[string]health.
		ComponentStatus {
		return h.parts
	}}
	probes.accessProven = func(string) bool { return h.proof }
	h.a = &selfAlerter{conditions: probes.conditions,
		proven: probes.provenParts, sink: h.sent,
		store: store, cluster: "shop-eu", updates: updates}
	return h
}

func (h *selfHarness) degrade(part, reason string) {
	h.parts[part] = health.ComponentStatus{State: "degraded",
		Reason: reason}
}

// steps runs a check every 30 s from `from` for d.
func (h *selfHarness) steps(from time.Time, d time.Duration) time.Time {
	at := from
	for ; !at.After(from.Add(d)); at = at.Add(selfAlertEvery) {
		h.a.step(at)
	}
	return at
}

var selfT0 = time.Date(2026, 11, 1, 10, 12, 0, 0, time.UTC)

func TestSelfAlertWaitsForTheHoldThenSendsOneQuietMessage(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	h.degrade("kubelet-stats", "kubelet_partially_unreachable")

	at := h.steps(selfT0, 9*time.Minute)
	require.Empty(t, h.sent.msgs, "nothing before the hold")
	h.steps(at, 30*time.Minute)

	require.Len(t, h.sent.msgs, 1, "one message, not one per check")
	m := h.sent.msgs[0]
	assert.Equal(t, "kwatch-self/kubelet-stats", m.Key)
	assert.True(t, m.Opens)
	assert.True(t, m.SkipPaging, "never paged")
	assert.Equal(t, "info", m.Route.Severity)
	assert.Equal(t, []string{SelfAlertReason}, m.Route.Reasons)
	assert.Equal(t, notification.StatusLow, m.Status)
	assert.Contains(t, m.Note, "kwatch in shop-eu is degraded: it "+
		"cannot read usage from some nodes' kubelets since 10:12 UTC.")
	assert.Contains(t, m.Note, "What this means: ")
	assert.Contains(t, m.Note, "What to do: ")
}

func TestSelfAlertUpdatesInPlaceAndSaysBackToNormal(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	h.degrade("kubelet-stats", "kubelet_partially_unreachable")
	at := h.steps(selfT0, 11*time.Minute)
	h.degrade("kubelet-stats", "kubelet_unreachable")
	at = h.steps(at, 5*time.Minute)
	delete(h.parts, "kubelet-stats")
	h.steps(at, 5*time.Minute)

	require.Len(t, h.sent.msgs, 3)
	update, closed := h.sent.msgs[1], h.sent.msgs[2]
	assert.Equal(t, 2, update.Revision)
	assert.Equal(t, h.sent.msgs[0].Key, update.Key, "same conversation")
	assert.Contains(t, update.Note, "any node's kubelet")
	assert.Equal(t, 3, closed.Revision)
	assert.Equal(t, notification.StatusResolved, closed.Status)
	assert.Contains(t, closed.Note, "is back to normal after")
	assert.Empty(t, h.store.state.Open)
}

func TestSelfAlertWithoutThreadsSendsNoUpdates(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, false)
	h.degrade("kubelet-stats", "kubelet_partially_unreachable")
	at := h.steps(selfT0, 11*time.Minute)
	h.degrade("kubelet-stats", "kubelet_unreachable")
	h.steps(at, 5*time.Minute)

	require.Len(t, h.sent.msgs, 1)
	assert.Equal(t, "kubelet_unreachable",
		h.store.state.Open["kubelet-stats"].Reason, "kept for the close")
}

func TestSelfAlertDailyCap(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	for i := 0; i < selfAlertsPerDay+3; i++ {
		h.degrade(fmt.Sprintf("source-kind%d", i), "permission_denied")
	}
	at := h.steps(selfT0, 20*time.Minute)
	require.Len(t, h.sent.msgs, selfAlertsPerDay)

	// The next UTC day the rest may go out.
	h.steps(at.Add(14*time.Hour), time.Minute)
	assert.Len(t, h.sent.msgs, selfAlertsPerDay+3)
}

func TestSelfAlertStaysQuietForNormalStates(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	h.parts["heartbeat"] = health.ComponentStatus{State: "off",
		Reason: "heartbeat_not_configured", Available: true}
	h.degrade("source-gateways", "optional_api_unavailable")
	h.degrade("source-secrets", "cache_sync_pending")
	h.degrade("leader-election", "leadership_lost")
	h.degrade("license", "license_expired")
	h.degrade("coverage", "permission_denied")
	h.degrade("config-overlay", "config_overlay_invalid")
	h.parts["pipeline"] = health.ComponentStatus{State: "running",
		Available: true}

	h.steps(selfT0, time.Hour)

	assert.Empty(t, h.sent.msgs)
}

// A restart neither repeats an open message nor closes it before the
// new session has run the hold: a night off is not a recovery.
func TestSelfAlertSurvivesARestart(t *testing.T) {
	store := &memSelfStore{}
	h := newSelfHarness(store, true)
	h.run("pod-logs")
	h.proof = true
	at0 := h.steps(selfT0, time.Minute)
	h.degrade("pod-logs", "optional_permission_denied")
	h.steps(at0, 11*time.Minute)
	require.Len(t, h.sent.msgs, 1)
	require.Contains(t, h.sent.msgs[0].Note, "refuses it the pod logs")

	morning := selfT0.Add(10 * time.Hour)
	again := newSelfHarness(store, true)
	at := again.steps(morning, 5*time.Minute)
	assert.Empty(t, again.sent.msgs, "not closed while starting")
	again.degrade("pod-logs", "optional_permission_denied")
	again.steps(at, time.Hour)
	assert.Empty(t, again.sent.msgs, "still open: not sent twice")

	delete(again.parts, "pod-logs")
	again.steps(morning.Add(2*time.Hour), time.Minute)
	require.Len(t, again.sent.msgs, 1)
	assert.Equal(t, notification.StatusResolved,
		again.sent.msgs[0].Status)
	assert.True(t, strings.HasPrefix(again.sent.msgs[0].Note,
		notification.MarkerResolved))
}

func TestSelfProbesSeeDecisionLagAndABacklog(t *testing.T) {
	p := &selfProbes{backlog: func() int64 { return 2 }}
	got := p.conditions(selfT0)
	require.Equal(t, []selfCondition{{Part: "delivery-backlog",
		Reason: selfReasonBacklog}}, got)
	reg := &metrics.Registry{}
	lag := &selfProbes{lag: &reg.DecisionLag}
	assert.Empty(t, lag.conditions(selfT0), "no decision is no lag")
	reg.ObserveDecisionLag(30)
	reg.ObserveDecisionLag(20)
	assert.Equal(t, []selfCondition{{Part: "decision-lag",
		Reason: selfReasonLag}}, lag.conditions(selfT0))
	reg.ObserveDecisionLag(0.01)
	assert.Empty(t, lag.conditions(selfT0), "caught up")
	assert.False(t, selfReportable("dashboard-tls", "component_failed"))
	assert.True(t, selfReportable("provider-slack", "provider_rejected"))
	assert.Contains(t, wordsOf("provider-slack", "provider_rejected").
		cannot, "the slack alert channel")
}
