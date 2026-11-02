package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
)

type fakeResolver map[string][]string

func (r fakeResolver) LookupHost(
	_ context.Context, host string,
) ([]string, error) {
	if answers, ok := r[host]; ok {
		return answers, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host,
		IsNotFound: true}
}

// dialLog records the addresses a dial reached and refuses them all, so
// no test leaves the machine.
type dialLog struct {
	mu   sync.Mutex
	seen []string
}

var errTestDial = errors.New("test dial")

func (d *dialLog) dial(
	_ context.Context, _, address string,
) (net.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = append(d.seen, address)
	return nil, errTestDial
}

func TestMetadataAddressBlocksOnlyMetadataRanges(t *testing.T) {
	cases := map[string]bool{
		"169.254.169.254":        true,
		"169.254.10.1":           true,
		"fd00:ec2::254":          true,
		"100.100.100.200":        true,
		"fe80::1":                true,
		"::ffff:169.254.169.254": true,
		"64:ff9b::a9fe:a9fe":     true,
		"10.0.0.5":               false,
		"192.168.1.10":           false,
		"172.16.0.1":             false,
		"127.0.0.1":              false,
		"::1":                    false,
		"100.100.100.201":        false,
		"fd00:ec2::253":          false,
		"8.8.8.8":                false,
		"2001:db8::1":            false,
		"not-an-address":         false,
	}
	for raw, want := range cases {
		if got := MetadataAddress(net.ParseIP(raw)); got != want {
			t.Errorf("MetadataAddress(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestGuardMetadataRefusesMetadataURLs(t *testing.T) {
	log := &dialLog{}
	client := guardWith(log, fakeResolver{})
	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://METADATA.GOOG./x",
		"http://[fd00:ec2::254]/latest",
		"http://100.100.100.200/latest",
		"http://[fe80::1%25eth0]:8080/x",
	} {
		_, err := client.Get(raw)
		if !errors.Is(err, ErrMetadataAddress) || !IsPermanent(err) {
			t.Errorf("%s: err = %v", raw, err)
		}
	}
	if len(log.seen) != 0 {
		t.Errorf("dialed %v", log.seen)
	}
}

func TestGuardMetadataRefusesANameResolvingToMetadata(t *testing.T) {
	log := &dialLog{}
	client := guardWith(log, fakeResolver{
		"hooks.example.test": {"10.0.0.9", "169.254.169.254"},
	})
	_, err := client.Post("http://hooks.example.test/x", "text/plain",
		strings.NewReader("hi"))
	if !errors.Is(err, ErrMetadataAddress) || !IsPermanent(err) {
		t.Errorf("err = %v", err)
	}
	if len(log.seen) != 0 {
		t.Errorf("dialed %v", log.seen)
	}
}

func TestGuardMetadataAllowsInClusterAndPrivateAddresses(t *testing.T) {
	log := &dialLog{}
	client := guardWith(log, fakeResolver{
		"mattermost.chat.svc.cluster.local": {"10.96.0.12"},
		"ntfy.local":                        {"192.168.1.20"},
	})
	for _, raw := range []string{
		"http://mattermost.chat.svc.cluster.local:8065/hooks/x",
		"http://ntfy.local/topic",
		"http://10.1.2.3/x",
		"http://192.168.0.4/x",
		"http://127.0.0.1:9/x",
	} {
		_, err := client.Get(raw)
		if !errors.Is(err, errTestDial) {
			t.Errorf("%s: err = %v, want the test dial", raw, err)
		}
	}
	want := []string{"10.96.0.12:8065", "192.168.1.20:80", "10.1.2.3:80",
		"192.168.0.4:80", "127.0.0.1:9"}
	if strings.Join(log.seen, " ") != strings.Join(want, " ") {
		t.Errorf("dialed %v, want %v", log.seen, want)
	}
}

func TestGuardMetadataReachesALoopbackServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
	defer server.Close()
	client := GuardMetadata(server.Client())
	sender := NewWithDependencies(Dependencies{HTTPClient: client,
		Clock: clock.RealClock{}})
	_, err := sender.Send(context.Background(), Request{Provider: "p",
		URL: server.URL})
	if err != nil {
		t.Fatalf("send = %v", err)
	}
	_, err = sender.Send(context.Background(), Request{Provider: "p",
		URL: "http://169.254.169.254/x"})
	if !errors.Is(err, ErrMetadataAddress) || !IsPermanent(err) {
		t.Errorf("metadata send = %v", err)
	}
	if strings.Contains(err.Error(), "/x") {
		t.Errorf("error quotes the path: %v", err)
	}
}

func TestGuardMetadataKeepsSDKClientAndTLSConfig(t *testing.T) {
	roots := &tls.Config{ServerName: "kept", MinVersion: tls.VersionTLS12}
	base := &http.Client{Transport: &http.Transport{
		TLSClientConfig: roots}}
	guarded := GuardMetadata(base)
	if got := (Dependencies{HTTPClient: guarded}).TLSConfig(); got == nil ||
		got.ServerName != "kept" {
		t.Errorf("TLSConfig = %+v", got)
	}
	_, err := SDKClient(guarded).Get("http://metadata.goog/x")
	if !errors.Is(err, ErrMetadataAddress) {
		t.Errorf("SDK client err = %v", err)
	}
	if GuardMetadata(nil) != nil {
		t.Error("a nil client must stay nil")
	}
}

func guardWith(log *dialLog, resolver Resolver) *http.Client {
	base := &http.Client{Transport: &http.Transport{
		DialContext: log.dial, Proxy: nil}}
	return guardMetadata(base, resolver)
}
