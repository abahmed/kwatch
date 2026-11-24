package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// kwatch is killed the moment delivery accepts an announcement: the
// outbox has it, and the batched save has not run. The record on disk
// must already say it was announced, or the restart announces it
// again while the outbox sends the first one too.
func TestAnnouncementIsSavedBeforeItIsHandedOver(t *testing.T) {
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	store := &memStore{}
	var onDisk []incident.Record
	handed := 0
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Store = store
		d.Sink = func(_ context.Context, dec incident.Decision,
			_ notification.Message) {
			if dec.Action != incident.Announce {
				return
			}
			handed++
			// The crash: what the store holds at the handoff.
			store.mu.Lock()
			onDisk = store.lastSaved
			store.mu.Unlock()
		}
	})
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("payments")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 0, 0
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, crashingPod("payments-1", rs.Name, "n1", start))
	h.run(start.Add(10*time.Minute), 10*time.Second)
	require.Equal(t, 1, handed, "announced once")

	restarted := incident.NewManager(incident.Config{IDNonce: harnessNonce},
		explain.NewSolver())
	restarted.Restore(onDisk, time.Time{})
	got := restarted.Incidents()
	require.Len(t, got, 1, "the record was on disk at the handoff")
	assert.NotEqual(t, incident.Settling, got[0].State,
		"restored as announced, not announced again")
	assert.False(t, got[0].Announced.IsZero())
}
