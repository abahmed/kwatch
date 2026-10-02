package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommandRejectsUnknownSubcommandArguments(t *testing.T) {
	tests := [][]string{
		{"lint", "--stirct"},
		{"lint", "--strict", "extra"},
		{"version", "--jsno"},
		{"version", "--json", "extra"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := runCommand(args, &out, &errOut, func() int {
				t.Fatal("server must not start")
				return 0
			})
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if !strings.Contains(errOut.String(), "unexpected arguments") {
				t.Fatalf("stderr = %q", errOut.String())
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", out.String())
			}
		})
	}
}

func TestParseLintArgsAcceptsDocumentedSpellings(t *testing.T) {
	strict, check, ok := parseLintArgs(
		[]string{"--strict", "check"})
	if !ok || !strict || !check {
		t.Fatalf("got strict=%v check=%v ok=%v", strict, check, ok)
	}
	if _, _, ok := parseLintArgs(nil); !ok {
		t.Fatal("no arguments must be accepted")
	}
}

func TestRunCommandVersionJSONStillAccepted(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runCommand([]string{"version", "--json"}, &out, &errOut,
		func() int { return 99 })
	if code != 0 || !strings.HasPrefix(out.String(), "{") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(),
			errOut.String())
	}
}
