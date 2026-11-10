package dashboard

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
)

// heldPort binds a free port and returns it with its listener.
func heldPort(t *testing.T) (int, net.Listener) {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port, ln
}

func portServer(port int) *Server {
	return NewServer(Options{Token: "tok", Port: port}, clock.RealClock{})
}

// A port that is taken at first is not the end: Open works once it is
// free.
func TestOpenCanBeTriedAgainAfterTheListenerFailed(t *testing.T) {
	port, holder := heldPort(t)
	s := portServer(port)
	if err := s.Open(context.Background()); err == nil {
		t.Fatal("Open bound a taken port")
	}
	_ = holder.Close()

	if err := s.Open(context.Background()); err != nil {
		t.Fatalf("Open after the port was freed: %v", err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// A key pair that cannot be read fails Open, before the port is bound,
// and Open works once the files are there.
func TestOpenReportsAnUnreadableKeyPairUntilItIsFixed(t *testing.T) {
	port, holder := heldPort(t)
	_ = holder.Close()
	dir := t.TempDir()
	s := NewServer(Options{
		Token: "tok", Port: port,
		TLSCertFile: filepath.Join(dir, "tls.crt"),
		TLSKeyFile:  filepath.Join(dir, "tls.key"),
	}, clock.RealClock{})
	if err := s.Open(context.Background()); err == nil {
		t.Fatal("Open accepted a missing key pair")
	}
	if s.listener != nil {
		t.Fatal("the port was bound without a key pair")
	}
	writePair(t, dir, "first", testEpoch)

	if err := s.Open(context.Background()); err != nil {
		t.Fatalf("Open with the key pair in place: %v", err)
	}
	_ = s.Stop(context.Background())
}

// A Serve that failed leaves a server that opens and serves again.
func TestServeCanRunAgainAfterItFailed(t *testing.T) {
	port, holder := heldPort(t)
	_ = holder.Close()
	s := portServer(port)
	if err := s.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = s.listener.Close() // the listener breaks under the server
	if err := s.Serve(context.Background()); err == nil {
		t.Fatal("Serve on a broken listener returned no error")
	}

	if err := s.Open(context.Background()); err != nil {
		t.Fatalf("Open after a failed Serve: %v", err)
	}
	served := make(chan error, 1)
	go func() { served <- s.Serve(context.Background()) }()
	resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/")
	if err != nil {
		t.Fatalf("the reopened dashboard does not answer: %v", err)
	}
	_ = resp.Body.Close()
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve after Stop = %v", err)
	}
}

// A retry that is still waiting must not open a listener after Stop.
func TestStopBeforeOpenKeepsTheServerClosed(t *testing.T) {
	port, holder := heldPort(t)
	_ = holder.Close()
	s := portServer(port)
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Open(context.Background()); err == nil {
		t.Fatal("Open after Stop bound the port")
	}
}
