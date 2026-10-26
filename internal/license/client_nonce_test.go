package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// signedCapture records the bodies of signed calls after checking each
// signature against pub over the raw body, as the Worker does.
func signedCapture(
	t *testing.T, pub ed25519.PublicKey,
) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			sig, err := base64.RawURLEncoding.DecodeString(
				r.Header.Get("X-Kwatch-Sig"))
			if err != nil || !ed25519.Verify(pub, raw, sig) {
				t.Errorf("%s: signature does not cover the body", r.URL.Path)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Errorf("body: %v", err)
			}
			mu.Lock()
			got = append(got, m)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
	t.Cleanup(srv.Close)
	return srv, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

func TestClientSignedCallsSendAFreshSignedNonce(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv, bodies := signedCapture(t, pub)
	c := &Client{Endpoint: srv.URL}
	req := SignedRequest{
		LID: "lid", ClusterUUID: "c", ClusterProof: "p", TS: testNow,
	}
	ctx := context.Background()
	_, _ = c.Renew(ctx, priv, req)
	_, _ = c.Renew(ctx, priv, req)
	_, _ = c.Deactivate(ctx, priv, req)
	seen := map[string]bool{}
	for i, b := range bodies() {
		n, _ := b["nonce"].(string)
		raw, err := base64.RawURLEncoding.DecodeString(n)
		if err != nil || len(raw) != 16 {
			t.Fatalf("call %d nonce = %q, want 16 bytes base64url", i, n)
		}
		if seen[n] {
			t.Fatalf("call %d reused nonce %q", i, n)
		}
		seen[n] = true
	}
	if len(seen) != 3 {
		t.Fatalf("calls = %d, want 3", len(seen))
	}
}
