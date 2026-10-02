package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestOutboxStoreRoundTripsInOrder(t *testing.T) {
	store := newOutboxStore(openTestStore(t))
	queued := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	records := []delivery.OutboxRecord{
		{Version: 1, ID: "00000000000000000002", Provider: "slack",
			Message: "second", Queued: queued},
		{Version: 1, ID: "00000000000000000001", Provider: "pagerduty",
			Incident: &notification.Message{Key: "pod/web", Revision: 3},
			Queued:   queued},
	}
	require.NoError(t, store.WriteOutbox(records, nil))

	loaded, err := store.LoadOutbox()

	require.NoError(t, err)
	require.Len(t, loaded, 2)
	require.Equal(t, "00000000000000000001", loaded[0].ID)
	require.Equal(t, 3, loaded[0].Incident.Revision)
	require.Equal(t, "second", loaded[1].Message)

	require.NoError(t, store.WriteOutbox(nil, []string{loaded[0].ID}))
	loaded, err = store.LoadOutbox()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
}

// busyStopper reports that the final outbox write is still running.
type busyStopper struct{}

func (busyStopper) Stop(context.Context) error { return delivery.ErrOutboxBusy }

func TestFinishActiveSessionKeepsStoreOpenForOutboxWrite(t *testing.T) {
	state := openTestStore(t)
	disk := diskState{store: state}
	threads := newThreadSaver(&fakeThreads{threads: sampleThreads}, disk)

	finishActiveSession(context.Background(), activeShutdown{
		supervisor: newComponentSupervisor(time.Now),
		delivery:   busyStopper{}, threads: threads,
		session: &recordingEnder{}, state: state, guard: &storeGuard{},
	})

	require.NoError(t, disk.put("probe", true), "store closed under writer")
}
