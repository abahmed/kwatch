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

// scriptedTransport answers each request with the next response.
type scriptedTransport struct {
	calls     int
	responses []func() *http.Response
}

func (s *scriptedTransport) RoundTrip(
	*http.Request,
) (*http.Response, error) {
	next := s.responses[min(s.calls, len(s.responses)-1)]
	s.calls++
	return next(), nil
}

func answer(status int, body string, headers ...string) *http.Response {
	h := http.Header{}
	for i := 0; i+1 < len(headers); i += 2 {
		h.Set(headers[i], headers[i+1])
	}
	return &http.Response{
		StatusCode: status, Header: h,
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func scriptedDiscord(t *testing.T, rt http.RoundTripper) *Discord {
	t.Helper()
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
	return d
}

// An answer that names a long global limit must not make the next send
// sleep inside the library: that sleep ignores the context, so it would
// hold the worker past its deadline and past shutdown.
func TestDiscordNextSendDoesNotSleepOutAGlobalLimit(t *testing.T) {
	rt := &scriptedTransport{responses: []func() *http.Response{
		func() *http.Response {
			return answer(http.StatusNoContent, "",
				"X-RateLimit-Global", "true",
				"X-RateLimit-Reset-After", "600",
				"X-RateLimit-Remaining", "0")
		},
		func() *http.Response { return answer(http.StatusNoContent, "") },
	}}
	d := scriptedDiscord(t, rt)
	if err := d.SendMessage(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.SendMessage(context.Background(), "two") }()
	select {
	case err := <-done:
		assert.NoError(t, err)
		assert.Equal(t, 2, rt.calls)
	case <-time.After(3 * time.Second):
		t.Fatal("the second send sleeps on the first answer's limit")
	}
}

// A 429 whose body is not JSON (a proxy's HTML page) is still a rate
// limit, and keeps the wait its Retry-After header names.
func TestDiscord429WithANonJSONBodyKeepsRetryAfter(t *testing.T) {
	rt := &scriptedTransport{responses: []func() *http.Response{
		func() *http.Response {
			return answer(http.StatusTooManyRequests,
				"<html>slow down</html>", "Retry-After", "42")
		},
	}}
	d := scriptedDiscord(t, rt)

	err := d.SendMessage(context.Background(), "hi")

	var limited *ratelimit.Error
	if !errors.As(err, &limited) {
		t.Fatalf("got %v, want a rate limit", err)
	}
	assert.Equal(t, 42*time.Second, limited.RetryAfter)
}

// A redirect is not followed and is not retried: the URL is wrong.
func TestDiscordRedirectIsPermanent(t *testing.T) {
	rt := &scriptedTransport{responses: []func() *http.Response{
		func() *http.Response {
			return answer(http.StatusFound, "", "Location", "https://y.test/")
		},
	}}
	d := scriptedDiscord(t, rt)

	err := d.SendMessage(context.Background(), "hi")

	assert.True(t, transport.IsPermanent(err), "got %v", err)
	assert.ErrorContains(t, err, "check the configured URL")
	assert.Equal(t, 1, rt.calls)
}
