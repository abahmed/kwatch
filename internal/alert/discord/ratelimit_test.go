package discord

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

type limitedTransport struct{ calls int }

func (l *limitedTransport) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	l.calls++
	h := http.Header{}
	h.Set("Retry-After", "7")
	h.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header:     h,
		Body: io.NopCloser(strings.NewReader(
			`{"message":"slow","retry_after":7,"global":false}`)),
	}, nil
}

// A 429 must come back at once as a ratelimit.Error; discordgo must not
// sleep and retry on its own.
func TestDiscord429SurfacesWithoutLibraryRetry(t *testing.T) {
	rt := &limitedTransport{}
	d := NewDiscord(
		map[string]interface{}{"webhook": "https://x.test/api/w/1/tok"},
		"dev",
		transport.Dependencies{
			HTTPClient: &http.Client{Transport: rt},
			Clock:      clock.RealClock{},
		},
	)
	if d == nil {
		t.Fatal("discord not built")
	}
	done := make(chan error, 1)
	go func() { done <- d.SendMessage(context.Background(), "hi") }()
	select {
	case err := <-done:
		var rle *ratelimit.Error
		assert.True(t, errors.As(err, &rle), "got %v", err)
		assert.Equal(t, 1, rt.calls)
	case <-time.After(3 * time.Second):
		t.Fatal("send blocked: library is sleeping on the 429")
	}
}
