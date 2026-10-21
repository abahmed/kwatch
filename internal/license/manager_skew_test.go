package license

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// skewedHarness runs the worker on a clock 20 minutes ahead of the
// manager, beyond the server's ±600s timestamp window.
func skewedHarness(t *testing.T) *harness {
	h := newHarness(t)
	_, priv := testEnv()
	srv := &fakeClock{t: testNow.Add(20 * time.Minute)}
	h.worker = newFakeWorker(t, srv, priv)
	h.m = h.newManager(h.key)
	return h
}

func TestManagerSignsWithServerCorrectedTime(t *testing.T) {
	h := skewedHarness(t)
	h.run() // activation learns the offset from srv_time
	if h.m.offset != 20*time.Minute {
		t.Fatalf("offset = %v, want 20m", h.m.offset)
	}
	h.m.cycle(context.Background())
	if n := h.worker.count("/license/renew"); n != 1 {
		t.Fatalf("renew requests = %d, want 1 with the offset", n)
	}
	if st := h.m.Status(); st.State != StateActive {
		t.Fatalf("state = %v", st.State)
	}
}

func TestManagerStaleTimestampResyncsAndRetriesOnce(t *testing.T) {
	h := skewedHarness(t)
	h.run()
	h.m.mu.Lock()
	h.m.offset = 0 // as if the clock drifted since the last answer
	h.m.mu.Unlock()
	if d := h.m.cycle(context.Background()); d != renewInterval {
		t.Fatalf("next delay = %v, want a normal interval", d)
	}
	if n := h.worker.count("/license/renew"); n != 2 {
		t.Fatalf("renew requests = %d, want stale then retry", n)
	}
}

func TestManagerRepeatedStaleTimestampKeepsTheState(t *testing.T) {
	h := newHarness(t)
	h.run()
	h.worker.set(func(p string) (int, map[string]any, map[string]string) {
		if p == "/license/renew" {
			return 400, map[string]any{"error": "stale_timestamp"}, nil
		}
		return 0, nil, nil
	})
	for range 3 {
		h.m.cycle(context.Background())
		if st := h.m.Status(); st.State != StateActive {
			t.Fatalf("stale timestamps changed the state: %+v", st)
		}
	}
	if n := h.worker.count("/license/renew"); n != 6 {
		t.Fatalf("renew requests = %d, want one retry per cycle", n)
	}
}

func TestClientReadsServerTimeHeaderOn204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(srvTimeHeader, "1700000000")
			w.WriteHeader(http.StatusNoContent)
		}))
	defer srv.Close()
	_, priv := testEnv()
	c := &Client{Endpoint: srv.URL, HTTP: srv.Client()}
	res, err := c.Ping(context.Background(), priv,
		SignedRequest{LID: "lic_1", TS: testNow}, Ping{Checkins: true})
	if err != nil || !res.SrvTime.Equal(time.Unix(1700000000, 0)) {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestClientActivateRetriesAReusedNonce(t *testing.T) {
	h := newHarness(t)
	calls := 0
	h.worker.set(func(p string) (int, map[string]any, map[string]string) {
		if p != "/license/activate" {
			return 0, nil, nil
		}
		if calls++; calls == 1 {
			return 409, map[string]any{"error": "nonce_reused"}, nil
		}
		return 0, nil, nil
	})
	h.run()
	if n := h.worker.count("/license/activate"); n != 2 {
		t.Fatalf("activate requests = %d, want 2", n)
	}
	if st := h.m.Status(); st.State != StateActive {
		t.Fatalf("state = %v", st.State)
	}
	if ts, _ := h.worker.lastActive["ts"].(float64); int64(ts) !=
		testNow.Unix() {
		t.Fatalf("activate ts = %v, want the corrected time", ts)
	}
}
