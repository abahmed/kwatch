package license

import (
	"context"
	"testing"
	"time"
)

func TestManagerSubscribeOnlyOnChange(t *testing.T) {
	h := newHarness(t)
	ch := h.m.Subscribe()
	h.m.init(context.Background())
	if st := <-ch; st.State != StatePending {
		t.Fatalf("first = %+v", st)
	}
	for i := 0; i < 5; i++ {
		h.m.Status()
	}
	select {
	case st := <-ch:
		t.Fatalf("unexpected emit %+v", st)
	default:
	}
	h.m.cycle(context.Background())
	if st := <-ch; st.State != StateActive {
		t.Fatalf("got %+v", st)
	}
	// Same state again (renew) does not emit.
	h.clock.Advance(25 * time.Hour)
	h.m.cycle(context.Background())
	select {
	case st := <-ch:
		t.Fatalf("unexpected emit %+v", st)
	default:
	}
	// Coalescing: two changes with no reader leave only the latest.
	h.setUID("other")
	h.m.Status() // invalid
	h.setUID("kube-uid")
	h.m.Status() // active
	h.setUID("other")
	h.m.Status() // invalid
	if len(ch) != 1 {
		t.Fatalf("buffered = %d", len(ch))
	}
	if st := <-ch; st.State != StateInvalid {
		t.Fatalf("latest = %+v", st)
	}
}

func TestManagerKubeSystemUIDChange(t *testing.T) {
	h := newHarness(t)
	h.run()
	h.setUID("cloned-cluster-uid")
	st := h.m.Status()
	if st.State != StateInvalid || st.Plan == "" {
		t.Fatalf("status = %+v", st)
	}
	if st.State.ProEnabled() {
		t.Fatal("clone must not enable Pro")
	}
	// The server also refuses, because the proof differs.
	h.worker.set(func(p string) (int, map[string]any, map[string]string) {
		return 403, map[string]any{"error": "cluster_proof_mismatch"}, nil
	})
	h.m.cycle(context.Background())
	if h.m.Status().State != StateConflict {
		t.Fatalf("status = %+v", h.m.Status())
	}
}

func TestManagerRejectsLeaseForOtherBinding(t *testing.T) {
	h := newHarness(t)
	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		// bound to whatever the fake has (empty)
		l := h.worker.lease("lic_abcdef1234")
		return 200, map[string]any{"lease": l}, nil
	})
	h.run()
	if h.leases.lease != "" {
		t.Fatal("lease that does not match the binding was saved")
	}
	if st := h.m.Status(); st.State != StatePending {
		t.Fatalf("status = %+v", st)
	}
}

func TestManagerDeactivate(t *testing.T) {
	h := newHarness(t)
	h.run()
	if err := h.m.Deactivate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.worker.count("/v1/license/deactivate") != 1 ||
		h.worker.bound || h.worker.badSigs != 0 {
		t.Fatal("deactivate not accepted")
	}
	if h.leases.lease != "" {
		t.Fatal("lease kept")
	}
	before := h.worker.total()
	h.m.cycle(context.Background())
	if h.worker.total() != before {
		t.Fatal("released manager contacted the server")
	}
}

func TestManagerLoop(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	h.m.cfg.Sleep = func(ctx context.Context, d time.Duration) error {
		h.sleeps = append(h.sleeps, d)
		n++
		if n == 3 {
			cancel()
		}
		return ctx.Err()
	}
	h.m.Start(ctx)
	h.m.Wait()
	if len(h.sleeps) != 3 || h.sleeps[0] != 0 || h.sleeps[1] != 24*time.Hour {
		t.Fatalf("sleeps = %v", h.sleeps)
	}
	if h.worker.count("/v1/license/activate") != 1 ||
		h.worker.count("/v1/license/renew") != 1 {
		t.Fatal("unexpected request counts")
	}
}
