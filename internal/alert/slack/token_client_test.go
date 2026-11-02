package slack

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// redirectingTransport answers every request with a redirect to another
// host and counts the requests it saw.
type redirectingTransport struct{ calls atomic.Int32 }

func (r *redirectingTransport) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	r.calls.Add(1)
	header := http.Header{}
	header.Set("Location", "https://elsewhere.invalid/api/chat.postMessage")
	return &http.Response{StatusCode: http.StatusFound, Header: header,
		Body: io.NopCloser(strings.NewReader(""))}, nil
}

// The token path runs slack-go's own requests; they must not follow a
// redirect, which would carry the bot token to another host.
func TestSlackTokenClientDoesNotFollowRedirects(t *testing.T) {
	rt := &redirectingTransport{}
	s := NewSlack(map[string]interface{}{
		"token": "xoxb-test", "channel": "#alerts",
	}, "dev", transport.Dependencies{
		HTTPClient: &http.Client{Transport: rt},
		Clock:      clock.RealClock{},
	})
	if s == nil {
		t.Fatal("slack was not constructed")
	}

	err := s.SendMessage(context.Background(), "hello")

	if err == nil {
		t.Fatal("a redirect must not count as delivered")
	}
	if got := rt.calls.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1 (no redirect followed)", got)
	}
}
