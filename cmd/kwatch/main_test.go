package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommandVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	called := false

	code := runCommand(
		[]string{"version"},
		&out,
		&errOut,
		func() int {
			called = true
			return 99
		},
	)

	if code != 0 {
		t.Fatalf("runCommand returned %d, want 0", code)
	}
	if called {
		t.Fatal("version command unexpectedly started the server")
	}
	if !strings.HasPrefix(out.String(), "version ") {
		t.Fatalf("version output = %q, want version prefix", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("version stderr = %q, want empty", errOut.String())
	}
}

func TestRunCommandRejectsUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	called := false

	code := runCommand(
		[]string{"unknown"},
		&out,
		&errOut,
		func() int {
			called = true
			return 7
		},
	)

	if code != 2 {
		t.Fatalf("unknown command returned %d, want 2", code)
	}
	if called {
		t.Fatal("unknown command unexpectedly called runApp")
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("error output = %q, want 'unknown command'",
			errOut.String())
	}
}
