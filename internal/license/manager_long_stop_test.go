package license

import (
	"context"
	"testing"
	"time"
)

// A staging cluster scaled to zero for more than a week starts with an
// expired cached lease; the first sync gets a new one and Pro is back
// before Start returns, with no operator action.
func TestManagerReturnsToProAfterALongStop(t *testing.T) {
	h := newHarness(t)
	h.run()
	if st := h.m.Status(); st.State != StateActive {
		t.Fatalf("state before the stop = %v", st.State)
	}
	syncs := h.worker.count("/license/activate") +
		h.worker.count("/license/renew")

	h.clock.Advance(8 * 24 * time.Hour)
	m := NewManager(Config{
		Key: h.key, Verifier: h.m.cfg.Verifier, Client: h.m.cfg.Client,
		InstanceStore: h.inst, LeaseStore: h.leases,
		ClusterUUID: "uuid-1", KubeSystemUID: h.m.cfg.KubeSystemUID,
		Version: "v1.2.3", Now: h.clock.Now, Sleep: h.m.cfg.Sleep,
		Jitter:             func(time.Duration) time.Duration { return 0 },
		InitialSyncTimeout: 5 * time.Second,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); m.Wait() }()
	m.Start(ctx)

	if st := m.Status(); st.State != StateActive {
		t.Fatalf("state after the stop = %v (%s)", st.State, st.Reason)
	}
	after := h.worker.count("/license/activate") +
		h.worker.count("/license/renew")
	if after <= syncs {
		t.Fatal("the restart did not contact the license server")
	}
}
