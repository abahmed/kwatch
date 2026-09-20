package ratelimit

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterAtSupportsSeconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{
		"Retry-After": []string{"7"},
	}}

	if got := ParseRetryAfterAt(resp, time.Time{}); got != 7*time.Second {
		t.Fatalf("retry delay = %s, want 7s", got)
	}
}

func TestParseRetryAfterAtUsesProvidedTimeForHTTPDate(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	resp := &http.Response{Header: http.Header{
		"Retry-After": []string{"Sat, 12 Sep 2026 12:00:05 GMT"},
	}}

	if got := ParseRetryAfterAt(resp, now); got != 5*time.Second {
		t.Fatalf("retry delay = %s, want 5s", got)
	}
}

func TestParseRetryAfterAtRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "-1", "not-a-date"} {
		resp := &http.Response{Header: http.Header{
			"Retry-After": []string{value},
		}}
		if got := ParseRetryAfterAt(resp, time.Time{}); got != 0 {
			t.Fatalf("retry delay for %q = %s, want zero", value, got)
		}
	}
}

func TestParseRetryAfterAtCapsVeryLargeSeconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{
		"Retry-After": []string{"9223372036854775807"},
	}}
	if got := ParseRetryAfterAt(resp, time.Time{}); got != 24*time.Hour {
		t.Fatalf("retry delay = %s, want 24h", got)
	}
}
