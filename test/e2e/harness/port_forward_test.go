//go:build e2e

package harness

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestReadForwardedPortParsesKubectlOutput(t *testing.T) {
	port, err := readForwardedPort(
		strings.NewReader("Forwarding from 127.0.0.1:49152 -> 8060\n"),
		strings.NewReader(""),
	)
	if err != nil {
		t.Fatal(err)
	}
	if port != 49152 {
		t.Fatalf("port = %d, want 49152", port)
	}
}

func TestReadForwardedPortRejectsClosedOutput(t *testing.T) {
	_, err := readForwardedPort(
		strings.NewReader("unable to listen on any requested ports\n"),
		strings.NewReader("error forwarding port\n"),
	)
	if err == nil {
		t.Fatal("closed port-forward output unexpectedly succeeded")
	}
}

// kubectl keeps writing to its pipes for the life of the forward. After
// the port is read, those writes must still be drained or kubectl stalls.
func TestReadForwardedPortKeepsDrainingAfterPort(t *testing.T) {
	outRead, outWrite := io.Pipe()
	errRead, errWrite := io.Pipe()
	defer outWrite.Close()
	defer errWrite.Close()
	go func() {
		_, _ = outWrite.Write(
			[]byte("Forwarding from 127.0.0.1:49152 -> 8060\n"))
	}()
	port, err := readForwardedPort(outRead, errRead)
	if err != nil || port != 49152 {
		t.Fatalf("port = %d, err = %v", port, err)
	}

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for i := 0; i < 50; i++ {
			_, _ = outWrite.Write([]byte("Handling connection for 49152\n"))
			_, _ = errWrite.Write([]byte("an error line\n"))
		}
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("output was not drained after the port was read")
	}
}
