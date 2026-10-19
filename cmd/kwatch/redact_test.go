package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRedactRedactsEveryLine(t *testing.T) {
	in := strings.NewReader("token=abcdef123456\n" +
		"webhook https://hooks.slack.com/x\nplain line\n")
	var out, errOut bytes.Buffer
	if code := runRedact(nil, in, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	got := out.String()
	for _, secret := range []string{"abcdef123456", "hooks.slack.com"} {
		if strings.Contains(got, secret) {
			t.Fatalf("%q left in %q", secret, got)
		}
	}
	if !strings.HasSuffix(got, "plain line\n") {
		t.Fatalf("plain text changed: %q", got)
	}
}

func TestRunRedactRejectsArgumentsAndOverlongLines(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runRedact([]string{"x"}, strings.NewReader(""),
		&out, &errOut); code != 2 {
		t.Fatalf("arguments: exit %d", code)
	}
	long := strings.Repeat("a", maxRedactLine+1)
	out.Reset()
	if code := runRedact(nil, strings.NewReader(long),
		&out, &errOut); code != 1 {
		t.Fatalf("overlong line: exit %d", code)
	}
	if out.Len() != 0 {
		t.Fatal("an overlong line must not be passed through")
	}
}
