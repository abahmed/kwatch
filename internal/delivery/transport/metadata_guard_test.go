package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/cloudmeta"
)

// dialLog records the addresses a guarded dialer was about to connect to
// and refuses them all, before any packet is sent, so no test leaves the
// machine. It runs after the metadata check.
type dialLog struct {
	mu   sync.Mutex
	seen []string
}

var errTestDial = errors.New("test dial")

func (d *dialLog) control(
	_ context.Context, _, address string, _ syscall.RawConn,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = append(d.seen, address)
	return errTestDial
}

// Every metadata address the shared list names is refused before any
// dial, by literal URL and by a name that resolves to it.
func TestGuardMetadataRefusesEverySharedMetadataAddress(t *testing.T) {
	log := &dialLog{}
	names := fakeResolver{}
	for i, ip := range cloudmeta.Addresses() {
		names[fmt.Sprintf("meta%d.example.test", i)] = []string{ip.String()}
	}
	client := guardWith(log, names)
	for i, ip := range cloudmeta.Addresses() {
		for _, raw := range []string{
			"http://" + net.JoinHostPort(ip.String(), "80") + "/x",
			fmt.Sprintf("http://meta%d.example.test/x", i),
		} {
			_, err := client.Get(raw)
			if !errors.Is(err, ErrMetadataAddress) || !IsPermanent(err) {
				t.Errorf("%s: err = %v", raw, err)
			}
		}
	}
	if len(log.seen) != 0 {
		t.Errorf("dialed %v", log.seen)
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

// The check runs on the address being connected: a name that resolves
// to a metadata address is refused there, and a metadata answer next to
// another one is never connected to.
func TestGuardMetadataRefusesANameResolvingToMetadata(t *testing.T) {
	log := &dialLog{}
	client := guardWith(log, fakeResolver{
		"hooks.example.test": {"169.254.169.254"},
		"mixed.example.test": {"10.0.0.9", "169.254.169.254"},
	})
	_, err := client.Post("http://hooks.example.test/x", "text/plain",
		strings.NewReader("hi"))
	if !errors.Is(err, ErrMetadataAddress) || !IsPermanent(err) {
		t.Errorf("err = %v", err)
	}
	if len(log.seen) != 0 {
		t.Errorf("dialed %v", log.seen)
	}
	_, _ = client.Get("http://mixed.example.test/x")
	if strings.Join(log.seen, " ") != "10.0.0.9:80" {
		t.Errorf("dialed %v, want only 10.0.0.9:80", log.seen)
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

func guardWith(log *dialLog, names fakeResolver) *http.Client {
	base := &http.Client{Transport: &http.Transport{Proxy: nil}}
	return guardMetadata(base, &net.Dialer{Resolver: names.resolver(),
		ControlContext: log.control})
}
