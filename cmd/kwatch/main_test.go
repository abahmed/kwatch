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
		strings.NewReader(""),
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
		strings.NewReader(""),
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

func TestRunReplayDryRun(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	var out, errOut bytes.Buffer
	input := `{"namespace":"dev","podName":"api",` +
		`"reason":"CrashLoopBackOff","events":"restarted"}` + "\n"

	code := runReplay(true, strings.NewReader(input), &out, &errOut)

	if code != 0 {
		t.Fatalf("runReplay returned %d, want 0", code)
	}
	if !strings.Contains(
		out.String(),
		"would replay to []: [replay] dev/api CrashLoopBackOff: restarted",
	) {
		t.Fatalf("dry-run output = %q, want replay summary", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("dry-run stderr = %q, want empty", errOut.String())
	}
}
