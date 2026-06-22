package llm

import (
	"strings"
	"testing"
)

func TestScrub(t *testing.T) {
	r := newRedactor()
	tests := []string{
		"password=hunter2",
		"Authorization: Bearer x",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
		"secret=my-secret-value",
		"token=abc.def.ghi",
	}
	for _, in := range tests {
		got := r.scrub(in)
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("not scrubbed: %q → %q", in, got)
		}
	}
}

func TestScrubNoFalsePositive(t *testing.T) {
	r := newRedactor()
	in := "normal log line with no secrets"
	got := r.scrub(in)
	if got != in {
		t.Errorf("unexpected transformation: %q → %q", in, got)
	}
}
