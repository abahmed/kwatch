package kube

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// namedAnswers is a Resolver with a fixed answer per name.
type namedAnswers map[string][]string

func (r namedAnswers) LookupHost(
	_ context.Context, host string,
) ([]string, error) {
	if answers, ok := r[host]; ok {
		return answers, nil
	}
	return nil, errors.New("no such host")
}

// dialLog records the addresses dialed and answers with a live
// connection, so a probe that reaches it reads as healthy.
type dialLog struct {
	mu        sync.Mutex
	addresses []string
}

func (d *dialLog) dial(
	_ context.Context, _, address string,
) (net.Conn, error) {
	d.mu.Lock()
	d.addresses = append(d.addresses, address)
	d.mu.Unlock()
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func (d *dialLog) dialed() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.addresses...)
}

func restrictedProber(
	resolver Resolver, dial *dialLog, client *http.Client,
) *ActiveProber {
	return NewActiveProber(ActiveProbeConfig{
		Now: fixedNow, Timeout: time.Second, Resolver: resolver,
		Dial: dial.dial, HTTPClient: client,
	})
}

func healthy(t *testing.T, target ProbeTarget, p *ActiveProber) bool {
	t.Helper()
	return isHealthy(p.check(context.Background(), target))
}

func isHealthy(observation inventory.Observation) bool {
	ok, known := observation.Attributes[AttrHealthy].AsBool()
	return known && ok
}

// In-cluster Services and pods have private addresses; probing them is
// what a KwatchConfig probe is for.
func TestRestrictedTCPTargetAllowsAClusterAddress(t *testing.T) {
	dial := &dialLog{}
	prober := restrictedProber(namedAnswers{
		"db.apps.svc": {"10.1.2.3"},
	}, dial, nil)

	for _, address := range []string{"db.apps.svc:5432", "10.1.2.3:5432",
		"192.168.1.5:80", "172.20.0.9:80", "100.64.1.1:80", "[fd00::1]:80"} {
		assert.True(t, healthy(t, ProbeTarget{Name: "db",
			Address: address, Restricted: true}, prober), address)
	}
	assert.Equal(t, "10.1.2.3:5432", dial.dialed()[0],
		"the connection goes to the address that was checked")
}

// A name can resolve to a sensitive address: the guard at overlay time
// only sees names, so the dial refuses it.
func TestRestrictedTCPTargetRefusesANameThatResolvesToASensitiveAddress(
	t *testing.T,
) {
	for _, answer := range []string{"169.254.169.254", "127.0.0.1", "::1",
		"fe80::1", "0.0.0.0", "224.0.0.1"} {
		dial := &dialLog{}
		prober := restrictedProber(namedAnswers{
			"db.example.com": {answer},
		}, dial, nil)

		observation := prober.check(context.Background(), ProbeTarget{
			Name: "db", Address: "db.example.com:5432", Restricted: true})

		assert.False(t, isHealthy(observation), answer)
		assert.Equal(t, "blocked: private address",
			observation.Attributes[AttrProbeError].AsText(), answer)
		assert.Empty(t, dial.dialed(), "nothing is dialed for %s", answer)
	}
}

func TestRestrictedTCPTargetRefusesAMixedAnswer(t *testing.T) {
	dial := &dialLog{}
	prober := restrictedProber(namedAnswers{
		"db.example.com": {"10.1.2.3", "169.254.169.254"},
	}, dial, nil)

	assert.False(t, healthy(t, ProbeTarget{Name: "db",
		Address: "db.example.com:5432", Restricted: true}, prober))
	assert.Empty(t, dial.dialed())
}

func TestRestrictedTCPTargetDialsTheAddressItChecked(t *testing.T) {
	dial := &dialLog{}
	prober := restrictedProber(namedAnswers{
		"db.example.com": {"93.184.216.34"},
	}, dial, nil)

	ok := healthy(t, ProbeTarget{Name: "db",
		Address: "db.example.com:5432", Restricted: true}, prober)

	assert.True(t, ok)
	assert.Equal(t, []string{"93.184.216.34:5432"}, dial.dialed(),
		"the connection goes to the address that was checked")
}

func TestRestrictedTCPTargetRefusesASensitiveAddressLiteral(t *testing.T) {
	dial := &dialLog{}
	prober := restrictedProber(namedAnswers{}, dial, nil)

	for _, address := range []string{"169.254.169.254:80", "127.0.0.1:80",
		"[::1]:80", "[fe80::1]:80", "0.0.0.0:80"} {
		assert.False(t, healthy(t, ProbeTarget{Name: "db",
			Address: address, Restricted: true}, prober), address)
	}
	assert.Empty(t, dial.dialed())
}

// A target the mounted configuration lists is trusted: it is dialed as
// written, whatever its name resolves to.
func TestMountedTCPTargetKeepsDialingItsOwnAddress(t *testing.T) {
	dial := &dialLog{}
	prober := restrictedProber(namedAnswers{
		"db.apps.svc": {"10.1.2.3"},
	}, dial, nil)

	ok := healthy(t, ProbeTarget{Name: "db",
		Address: "db.apps.svc:5432"}, prober)

	assert.True(t, ok)
	assert.Equal(t, []string{"db.apps.svc:5432"}, dial.dialed())
}

// Without an injected dial the prober's own dialer still refuses a
// sensitive address when the name resolves to one.
func TestRestrictedTCPTargetRefusesLoopbackAtConnectTime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	prober := NewActiveProber(ActiveProbeConfig{
		Now: fixedNow, Timeout: time.Second,
		Resolver: namedAnswers{},
	})

	observation := prober.check(context.Background(), ProbeTarget{
		Name: "local", Address: listener.Addr().String(),
		Restricted: true})

	assert.False(t, isHealthy(observation))
	assert.Equal(t, "blocked: private address",
		observation.Attributes[AttrProbeError].AsText())
}

func TestRestrictedHTTPTargetRefusesANameThatResolvesToLoopback(
	t *testing.T,
) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { hits++ }))
	defer server.Close()
	dial := &dialLog{}
	prober := restrictedProber(namedAnswers{
		"app.example.com": {"127.0.0.1"},
	}, dial, &http.Client{Transport: &http.Transport{}})

	ok := healthy(t, ProbeTarget{Name: "app",
		URL: "http://app.example.com/", Restricted: true}, prober)

	assert.False(t, ok)
	assert.Zero(t, hits)
	assert.Empty(t, dial.dialed())
}

func TestRestrictedHTTPTargetConnectsToTheAddressItChecked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	var dialed []string
	var mu sync.Mutex
	prober := NewActiveProber(ActiveProbeConfig{
		Now: fixedNow, Timeout: time.Second,
		Resolver: namedAnswers{"app.example.com": {"93.184.216.34"}},
		Dial: func(ctx context.Context, network, address string) (
			net.Conn, error,
		) {
			mu.Lock()
			dialed = append(dialed, address)
			mu.Unlock()
			return (&net.Dialer{}).DialContext(ctx, network,
				server.Listener.Addr().String())
		},
		HTTPClient: &http.Client{Transport: &http.Transport{}},
	})

	ok := healthy(t, ProbeTarget{Name: "app",
		URL: "http://app.example.com/", Restricted: true}, prober)

	assert.True(t, ok)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"93.184.216.34:80"}, dialed)
}

// A client whose transport cannot be restricted is never used for a
// restricted target.
func TestRestrictedHTTPTargetFailsClosedOnAnUnknownTransport(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("must not be used")
		})}
	prober := restrictedProber(namedAnswers{
		"app.example.com": {"93.184.216.34"},
	}, &dialLog{}, client)

	ok := healthy(t, ProbeTarget{Name: "app",
		URL: "http://app.example.com/", Restricted: true}, prober)

	assert.False(t, ok)
	assert.False(t, called)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
