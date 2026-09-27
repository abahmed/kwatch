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
