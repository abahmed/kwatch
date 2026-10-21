package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func pingServer(
	t *testing.T, pub ed25519.PublicKey, status int, body string,
	got *map[string]any,
) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			sig, err := base64.RawURLEncoding.DecodeString(
				r.Header.Get("X-Kwatch-Sig"))
			if r.URL.Path != "/license/ping" || err != nil ||
				!ed25519.Verify(pub, raw, sig) {
				t.Errorf("bad request %s", r.URL.Path)
			}
			if got != nil {
				_ = json.Unmarshal(raw, got)
			}
			if status == http.StatusTooManyRequests {
				w.Header().Set("Retry-After", "120")
			}
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}))
	t.Cleanup(srv.Close)
	return &Client{Endpoint: srv.URL}
}

func TestClientPingSendsTheContractBody(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	var got map[string]any
	c := pingServer(t, pub, http.StatusNoContent, "", &got)
	req := SignedRequest{LID: "lic_1", ClusterUUID: "cu", ClusterProof: "cp",
		TS: time.Unix(1_800_000_000, 0), Version: "v1.0.0", NodeCount: 3}

	_, err := c.Ping(context.Background(), priv, req, Ping{
		Checkins: true, Stopping: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"lid": "lic_1", "cluster_uuid": "cu", "cluster_proof": "cp",
		"ts": float64(1_800_000_000), "kwatch_version": "v1.0.0",
		"node_count": float64(3), "checkins": true, "state": "stopping",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %v, want %v (body %v)", k, got[k], v, got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected fields: %v", got)
	}
}

func TestClientPingClassifiesAnswers(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	for _, tc := range []struct {
		status int
		body   string
		stops  bool
		retry  bool
	}{
		{http.StatusUnauthorized, `{"error":"bad_signature"}`, true, false},
		{http.StatusForbidden, `{"error":"revoked"}`, true, false},
		{http.StatusForbidden, `{"error":"expired"}`, true, false},
		{http.StatusConflict, `{"error":"bound_to_other_cluster"}`,
			true, false},
		{http.StatusConflict, `{"error":"instance_superseded"}`,
			true, false},
		{http.StatusTooManyRequests, `{"error":"rate_limited"}`,
			false, true},
		{http.StatusServiceUnavailable, ``, false, true},
		{http.StatusBadRequest, `{"error":"bad_request"}`, false, false},
	} {
		c := pingServer(t, pub, tc.status, tc.body, nil)
		_, err := c.Ping(context.Background(), priv,
			SignedRequest{TS: time.Unix(1, 0)}, Ping{Checkins: true})
		if err == nil {
			t.Fatalf("%d: no error", tc.status)
		}
		if StopsCheckins(err) != tc.stops || IsTransient(err) != tc.retry {
			t.Fatalf("%d %s: stops=%v transient=%v (%v)", tc.status,
				tc.body, StopsCheckins(err), IsTransient(err), err)
		}
	}
}
