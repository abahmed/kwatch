package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestClientNoRedirects(t *testing.T) {
	var hits int
	target := httptest.NewServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { hits++ }))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redir.Close()
	c := &Client{Endpoint: redir.URL, HTTP: redir.Client()}
	_, err := c.Activate(context.Background(), ActivateRequest{Key: "k"})
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 307 {
		t.Fatalf("err = %v", err)
	}
	if hits != 0 {
		t.Fatal("redirect was followed")
	}
	if c.HTTP.CheckRedirect != nil {
		t.Fatal("caller's client was mutated")
	}
}

func TestClientErrors(t *testing.T) {
	status, body := 0, ""
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		ua = r.Header.Get("User-Agent")
		if status == 429 {
			w.Header().Set("Retry-After", "42")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := &Client{Endpoint: srv.URL + "/", HTTP: srv.Client()}
	call := func(s int, b string) error {
		status, body = s, b
		_, err := c.Activate(context.Background(), ActivateRequest{Key: "k"})
		return err
	}
	var conf ErrConflict
	conflict := `{"error":"bound_to_other_cluster",` +
		`"bound_since":1000,"srv_time":5}`
	if err := call(409, conflict); !errors.As(err, &conf) ||
		conf.BoundSince.Unix() != 1000 {
		t.Fatalf("conflict: %v", err)
	}
	var rl ErrRateLimited
	if err := call(429,
		`{"error":"rate_limited"}`); !errors.As(err, &rl) ||
		rl.RetryAfter != 42*time.Second || !IsTransient(err) {
		t.Fatalf("rate limit: %v", err)
	}
	if err := call(503, `oops`); !IsTransient(err) {
		t.Fatalf("5xx: %v", err)
	}
	if err := call(200, `not json`); !IsTransient(err) {
		t.Fatalf("bad 200: %v", err)
	}
	if err := call(400, `{"error":"invalid_key"}`); IsTransient(err) ||
		err == nil {
		t.Fatalf("400: %v", err)
	}
	for code, want := range map[string]error{
		"revoked": ErrRevoked, "suspended": ErrSuspended,
		"expired": ErrServerExpired, "trial_already_used": ErrTrialUsed,
		"key_replaced": ErrKeyReplaced,
	} {
		if err := call(403, `{"error":"`+code+`"}`); !errors.Is(err, want) ||
			IsTransient(err) {
			t.Fatalf("%s: %v", code, err)
		}
	}
	_, err := c.Activate(context.Background(), ActivateRequest{Key: "k"})
	if err == nil || ua != "kwatch-license/1" {
		t.Fatalf("ua = %q", ua)
	}
	status, body = 403, `{"error":"revoked","srv_time":77}`
	_, err = c.Activate(context.Background(), ActivateRequest{})
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked with srv_time: %v", err)
	}
	// Network failure is transient.
	srv.Close()
	if err := call(200, ""); !IsTransient(err) {
		t.Fatalf("network: %v", err)
	}
	if got := parseRetryAfter(strconv.Itoa(1 << 30)); got != 24*time.Hour {
		t.Fatalf("cap: %v", got)
	}
}

func TestEnsureInstanceKey(t *testing.T) {
	s := &memInstanceStore{}
	a, err := EnsureInstanceKey(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EnsureInstanceKey(context.Background(), s)
	if err != nil || !a.Equal(b) {
		t.Fatal("key not stable")
	}
	pub := PublicString(a.Public().(ed25519.PublicKey))
	raw, err := base64.RawURLEncoding.DecodeString(pub)
	if err != nil || len(raw) != 32 {
		t.Fatalf("pub = %q", pub)
	}
}
