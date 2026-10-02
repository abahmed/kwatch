package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

func TestSendRedactsSecretURLFromNetworkErrors(t *testing.T) {
	sender := NewWithDependencies(Dependencies{
		HTTPClient: &http.Client{Transport: failingRoundTripper{}},
		Clock:      clock.Func(time.Now),
	})
	_, err := sender.Send(context.Background(), Request{
		Provider: "Telegram",
		URL:      "https://api.telegram.org/bot123:SECRET/sendMessage?k=v",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SECRET") ||
		strings.Contains(err.Error(), "k=v") {
		t.Fatalf("error leaks URL secret: %v", err)
	}
	if !strings.Contains(err.Error(), "api.telegram.org") {
		t.Fatalf("error lost host: %v", err)
	}
}

func TestRedactedURLRejectsUnparsableValues(t *testing.T) {
	if got := redactedURL("::bad"); got != "[redacted]" {
		t.Fatalf("redactedURL = %q", got)
	}
}

func TestSendRedactsSecretURLFromRequestBuildErrors(t *testing.T) {
	sender := NewWithDependencies(Dependencies{
		HTTPClient: &http.Client{Transport: failingRoundTripper{}},
		Clock:      clock.Func(time.Now),
	})
	// A token pasted with a trailing newline makes the URL unparsable.
	_, err := sender.Send(context.Background(), Request{
		Provider: "Telegram",
		URL:      "https://api.telegram.org/bot123:SECRET\n/sendMessage",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error leaks URL secret: %v", err)
	}
	if !IsPermanent(err) {
		t.Fatalf("invalid URL must be permanent: %v", err)
	}
}

func TestLogURLKeepsOnlySchemeAndHost(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"path and query secrets", "https://hooks.example.org/T1/SECRET?k=v",
			"https://hooks.example.org"},
		{"user info", "https://user:pass@jira.example.org:8443/rest",
			"https://jira.example.org:8443"},
		{"no host", "not a url", "[invalid]"},
		{"unparseable", "http://[::1", "[invalid]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LogURL(tc.raw); got != tc.want {
				t.Fatalf("LogURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
