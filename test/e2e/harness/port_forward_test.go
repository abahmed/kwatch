//go:build e2e

package harness

import (
	"strings"
	"testing"
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
