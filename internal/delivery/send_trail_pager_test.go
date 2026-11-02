package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

func pagerAttempt(at int, kind, result string) SendAttempt {
	return SendAttempt{At: time.Unix(int64(at), 0), Provider: "pd",
		Kind: kind, Result: result, Pager: true}
}

func TestPagerStatesReadTheTrail(t *testing.T) {
	tests := []struct {
		name     string
		attempts []SendAttempt
		want     PagerState
	}{
		{"paged and closed", []SendAttempt{
			pagerAttempt(1, "incident", "ok"),
			pagerAttempt(2, "incident", "ok"),
			pagerAttempt(3, "resolve", "ok")},
			PagerState{Provider: "pd", State: PagerClosed,
				PagedAt: time.Unix(1, 0), ClosedAt: time.Unix(3, 0)}},
		{"still open after a failed resolve", []SendAttempt{
			pagerAttempt(1, "incident", "ok"),
			pagerAttempt(2, "resolve", "retrying")},
			PagerState{Provider: "pd", State: PagerOpen,
				PagedAt: time.Unix(1, 0)}},
		{"page failed", []SendAttempt{
			pagerAttempt(1, "incident", "failed")},
			PagerState{Provider: "pd", State: PagerFailed}},
		{"still retrying", []SendAttempt{
			pagerAttempt(1, "incident", "retrying")},
			PagerState{Provider: "pd", State: PagerPending}},
		{"reopened after close", []SendAttempt{
			pagerAttempt(1, "incident", "ok"),
			pagerAttempt(2, "resolve", "ok"),
			pagerAttempt(3, "incident", "ok")},
			PagerState{Provider: "pd", State: PagerOpen,
				PagedAt: time.Unix(3, 0)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PagerStates(tc.attempts)
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0])
		})
	}
}

// Chat sends are not pager states.
func TestPagerStatesSkipChat(t *testing.T) {
	chat := SendAttempt{Provider: "slack", Kind: "incident",
		Result: resultOK}
	assert.Empty(t, PagerStates([]SendAttempt{chat}))
}

// Sends to a pager are marked in the trail, so PagerStatus can say the
// alert is open there and when the pager refused the resolve.
func TestManagerPagerStatusFromDispatch(t *testing.T) {
	pager := &skippingProvider{errorRecorderProvider{name: "Pager"}}
	am := newTestManager()
	setManagerEntries(am, pagerEntryFor(pager))
	entry := managerEntries(am)[0]
	ctx := context.Background()

	_ = am.dispatch(ctx, &entry, incidentJob("a", "ns"), deliverOpts{})
	pager.err = errors.New("boom")
	resolve := incidentJob("a", "ns")
	resolve.incident.Status = notification.StatusResolved
	_ = am.dispatch(ctx, &entry, resolve, deliverOpts{})

	got := am.PagerStatus("a")
	require.Len(t, got, 1)
	assert.Equal(t, PagerOpen, got[0].State)
	assert.False(t, got[0].PagedAt.IsZero())
	assert.Equal(t, "failed, retrying", got[0].Reason)

	pager.err = nil
	_ = am.dispatch(ctx, &entry, resolve, deliverOpts{})
	assert.Equal(t, PagerClosed, am.PagerStatus("a")[0].State)
}
