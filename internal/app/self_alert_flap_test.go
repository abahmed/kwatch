package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

// A reason that flaps on every check edits the open message at most
// once per selfAlertHold, and the close still tells the newest reason.
func TestSelfAlertUpdatesOfAFlappingReasonAreSpaced(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	h.degrade("kubelet-stats", "kubelet_partially_unreachable")
	at := h.steps(selfT0, 11*time.Minute)
	require.Len(t, h.sent.msgs, 1, "opened")

	reasons := []string{"kubelet_unreachable",
		"kubelet_partially_unreachable"}
	latest, held := "", 0
	for end, i := at.Add(time.Hour), 0; at.Before(end); i++ {
		latest = reasons[i%2]
		h.degrade("kubelet-stats", latest)
		h.a.step(at)
		at = at.Add(selfAlertEvery)
		open := h.store.state.Open["kubelet-stats"]
		if open.Pending != "" {
			held++
			require.Equal(t, latest, open.Pending, "newest kept")
		} else {
			require.Equal(t, latest, open.Reason, "newest told")
		}
	}
	updates := len(h.sent.msgs) - 1
	assert.LessOrEqual(t, updates, 6, "one edit per hold at most")
	assert.GreaterOrEqual(t, updates, 4, "changes are still told")
	assert.Positive(t, held, "some reasons waited")

	delete(h.parts, "kubelet-stats")
	h.a.step(at)
	closed := h.sent.msgs[len(h.sent.msgs)-1]
	assert.Equal(t, notification.StatusResolved, closed.Status)
	assert.Contains(t, closed.Note, wordsOf("kubelet-stats", latest).cannot,
		"the close carries the newest reason")
	assert.Equal(t, updates+2, closed.Revision)
	assert.Empty(t, h.store.state.Open)
}

// A reason held back by the spacing is not lost: the close carries it.
func TestSelfAlertCloseCarriesAHeldBackReason(t *testing.T) {
	h := newSelfHarness(&memSelfStore{}, true)
	h.degrade("kubelet-stats", "kubelet_partially_unreachable")
	at := h.steps(selfT0, 11*time.Minute)
	h.degrade("kubelet-stats", "kubelet_unreachable")
	h.a.step(at)
	h.degrade("kubelet-stats", "kubelet_partially_unreachable")
	h.a.step(at.Add(selfAlertEvery))
	require.Len(t, h.sent.msgs, 2, "the first change is told at once")
	assert.Equal(t, "kubelet_partially_unreachable",
		h.store.state.Open["kubelet-stats"].Pending)

	delete(h.parts, "kubelet-stats")
	h.a.step(at.Add(2 * selfAlertEvery))
	require.Len(t, h.sent.msgs, 3)
	closed := h.sent.msgs[2]
	assert.Equal(t, 3, closed.Revision)
	assert.Contains(t, closed.Note, "some nodes' kubelets")
}
