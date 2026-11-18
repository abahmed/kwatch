package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/notification"
)

func (h *selfHarness) run(part string) {
	h.parts[part] = health.ComponentStatus{State: "running",
		Available: true}
}

func TestSelfAlertNeverGrantedOptionalPermissionStaysQuiet(t *testing.T) {
	store := &memSelfStore{}
	h := newSelfHarness(store, true)
	h.degrade("source-secrets", "optional_permission_denied")
	h.steps(selfT0, 6*time.Hour)
	assert.Empty(t, h.sent.msgs)

	again := newSelfHarness(store, true)
	again.degrade("source-secrets", "optional_permission_denied")
	again.steps(selfT0.Add(10*time.Hour), 3*time.Hour)
	assert.Empty(t, again.sent.msgs, "a restart does not change it")
}

func TestSelfAlertPodLogsRunningWithoutAReadIsNotGranted(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	h.run("pod-logs")
	at := h.steps(selfT0, 5*time.Minute)
	h.degrade("pod-logs", "optional_permission_denied")
	h.steps(at, time.Hour)
	assert.Empty(t, h.sent.msgs, "no read ever worked")
}

func TestSelfAlertGrantedThenRevokedSendsOneThenRecovers(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	h.run("source-secrets")
	at := h.steps(selfT0, time.Minute)
	h.degrade("source-secrets", "optional_permission_denied")
	at = h.steps(at, 9*time.Minute)
	require.Empty(t, h.sent.msgs, "held first")
	at = h.steps(at, time.Hour)
	require.Len(t, h.sent.msgs, 1)
	assert.True(t, h.sent.msgs[0].Opens)

	h.run("source-secrets")
	h.steps(at, time.Minute)
	require.Len(t, h.sent.msgs, 2)
	assert.Equal(t, notification.StatusResolved, h.sent.msgs[1].Status)
}

func TestSelfAlertGrantSurvivesARestartAndAReset(t *testing.T) {
	store := &memSelfStore{}
	h := newSelfHarness(store, true)
	h.run("source-secrets")
	h.steps(selfT0, time.Minute)
	require.True(t, store.state.Granted["source-secrets"])

	again := newSelfHarness(store, true)
	again.degrade("source-secrets", "optional_permission_denied")
	again.steps(selfT0.Add(8*time.Hour), time.Hour)
	assert.Len(t, again.sent.msgs, 1, "still treated as revoked")

	reset := newSelfHarness(&memSelfStore{}, true)
	reset.degrade("source-secrets", "optional_permission_denied")
	reset.steps(selfT0.Add(20*time.Hour), time.Hour)
	assert.Empty(t, reset.sent.msgs, "a reset starts fresh")
}

func TestSelfAlertRequiredAndOtherFailuresAreUnchanged(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	h.degrade("source-pods", "permission_denied")
	h.degrade("source-nodes", "cache_sync_timeout")
	h.steps(selfT0, 11*time.Minute)
	assert.Len(t, h.sent.msgs, 2, "never-granted rule is optional only")
}
