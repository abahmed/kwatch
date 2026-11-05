package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// The update that lowers an eased page goes to the providers that keep
// alerts by key only: chat is not told, and it is not carried by any
// chat message, so a cluster without a pager logs it as paging only.
func TestTierLoweredUpdateIsPagingOnly(t *testing.T) {
	e, seen := holdsHarness(t, incident.Notify, "n")
	d := decisionOf(e, "n", incident.Update)
	d.Reason = incident.ReasonTierLowered
	e.announcer.send(context.Background(), d, holdsNow)
	require.Len(t, *seen, 1)
	m := (*seen)[0]
	require.True(t, m.PagingOnly, "for the pagers only")
	require.Empty(t, m.ChatCarrier, "no chat message carries it")
	require.Contains(t, m.ShortText(), "no longer urgent")
}
