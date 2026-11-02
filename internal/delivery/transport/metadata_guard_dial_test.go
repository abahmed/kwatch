package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// A dual-stack name whose IPv6 path is blackholed still connects over
// IPv4, after the dialer's own fallback delay rather than a full dial
// timeout per address.
func TestGuardMetadataKeepsHappyEyeballs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
	defer server.Close()
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var tried []string
	blackhole := func(ctx context.Context, _, address string,
		_ syscall.RawConn) error {
		mu.Lock()
		tried = append(tried, address)
		mu.Unlock()
		if strings.HasPrefix(address, "[") {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	const fallback = 100 * time.Millisecond
	names := fakeResolver{"dual.example.test": {"::1", "127.0.0.1"}}
	client := guardMetadata(&http.Client{Transport: &http.Transport{}},
		&net.Dialer{Timeout: 30 * time.Second, FallbackDelay: fallback,
			Resolver: names.resolver(), ControlContext: blackhole})

	start := time.Now()
	resp, err := client.Get("http://dual.example.test:" + port + "/")
	took := time.Since(start)
	if err != nil {
		t.Fatalf("get = %v (tried %v)", err, tried)
	}
	_ = resp.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(tried) < 2 || !strings.HasPrefix(tried[0], "[::1]") {
		t.Errorf("tried %v, want IPv6 first, then IPv4", tried)
	}
	if took < fallback || took > 5*time.Second {
		t.Errorf("took %v, want the %v fallback, not a dial timeout",
			took, fallback)
	}
}

// The proxy is the operator's own address and is dialed even when it is
// link-local; the target URL is still checked, and a request that does
// not go through the proxy is checked when it connects.
func TestGuardMetadataAllowsALinkLocalProxy(t *testing.T) {
	proxy, err := url.Parse("http://169.254.1.1:3128")
	if err != nil {
		t.Fatal(err)
	}
	log := &dialLog{}
	names := fakeResolver{"direct.example.test": {"169.254.169.254"}}
	client := guardMetadata(&http.Client{Transport: &http.Transport{
		Proxy: func(r *http.Request) (*url.URL, error) {
			if r.URL.Hostname() == "direct.example.test" {
				return nil, nil
			}
			return proxy, nil
		}}}, &net.Dialer{Resolver: names.resolver(),
		ControlContext: log.control})

	_, err = client.Get("http://hooks.example.test/x")
	if !errors.Is(err, errTestDial) {
		t.Errorf("proxied send = %v, want the test dial", err)
	}
	for _, raw := range []string{"http://169.254.169.254/latest",
		"http://metadata.google.internal/x",
		"http://direct.example.test/x"} {
		_, err = client.Get(raw)
		if !errors.Is(err, ErrMetadataAddress) || !IsPermanent(err) {
			t.Errorf("%s: err = %v", raw, err)
		}
	}
	if strings.Join(log.seen, " ") != "169.254.1.1:3128" {
		t.Errorf("dialed %v, want only the proxy", log.seen)
	}
}

// The production dialer keeps the client's timeouts and the default
// Happy Eyeballs fallback.
func TestGuardMetadataDialerKeepsTimeouts(t *testing.T) {
	d := metadataDialer(guardDialer())
	if d.Timeout != 30*time.Second || d.KeepAlive != 30*time.Second ||
		d.FallbackDelay != 0 || d.ControlContext == nil {
		t.Errorf("dialer = %+v", d)
	}
}
