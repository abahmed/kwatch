package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
)

// A redirect is not followed: a POST turned into a GET would count as
// delivered, and a redirect to another host would carry the auth headers
// there. It is a permanent error with a clear message.
func TestSendDoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307} {
		var hits atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(
			func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
		redirect := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL, status)
			}))

		_, err := NewWithDependencies(Dependencies{
			HTTPClient: &http.Client{}, Clock: clock.RealClock{},
		}).Send(context.Background(), Request{
			Provider: "test", URL: redirect.URL,
			Headers: map[string]string{"X-Auth": "secret"},
		})
		target.Close()
		redirect.Close()

		if err == nil || !IsPermanent(err) ||
			!strings.Contains(err.Error(), "redirected") {
			t.Fatalf("status %d: err = %v, want permanent redirect error",
				status, err)
		}
		if hits.Load() != 0 {
			t.Fatalf("status %d: the redirect target was called", status)
		}
	}
}

// A huge response body is read and drained only up to a limit.
func TestSendBoundsTheDrainOfALargeBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			chunk := make([]byte, 1<<16)
			for range 100 {
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		}))
	defer server.Close()

	body, err := NewWithDependencies(Dependencies{
		HTTPClient: &http.Client{}, Clock: clock.RealClock{},
	}).Send(context.Background(), Request{Provider: "test", URL: server.URL})
	if err != nil && !strings.Contains(err.Error(), "drain") {
		t.Fatal(err)
	}
	if len(body) != 1<<20 {
		t.Fatalf("body = %d bytes, want the 1 MiB cap", len(body))
	}
}
