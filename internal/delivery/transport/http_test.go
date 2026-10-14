package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSendClassifiesRateLimitResponse(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"busy"}`))
		}),
	)
	defer server.Close()

	_, err := NewWithDependencies(Dependencies{
		HTTPClient: http.DefaultClient,
		Clock:      clock.RealClock{},
	}).Send(context.Background(), Request{
		Provider: "test", URL: server.URL,
	})
	if err == nil {
		t.Fatal("Send returned nil error for a rate-limited response")
	}
	var rateLimitErr *ratelimit.Error
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("Send error = %T, want *ratelimit.Error", err)
	}
	if rateLimitErr.RetryAfter.Seconds() != 7 {
		t.Fatalf("RetryAfter = %s, want 7s", rateLimitErr.RetryAfter)
	}
}

func TestClassifyHTTPStatusMarksClientErrorsPermanent(t *testing.T) {
	err := errors.New("bad request")
	if got := ClassifyHTTPStatus(http.StatusBadRequest, err); got == err {
		t.Fatal("ClassifyHTTPStatus returned the unclassified error")
	}
	if got := ClassifyHTTPStatus(http.StatusBadGateway, err); got != err {
		t.Fatal("server errors should remain retryable")
	}
}

func TestSendUsesExplicitClient(t *testing.T) {
	called := false
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}
	body, err := NewWithDependencies(Dependencies{
		HTTPClient: client,
		Clock:      clock.RealClock{},
	}).Send(context.Background(), Request{
		Provider: "test", URL: "http://injected",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !called {
		t.Fatal("Send did not use the injected HTTP client")
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestSendHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	}

	_, err := NewWithDependencies(Dependencies{
		HTTPClient: client,
		Clock:      clock.RealClock{},
	}).Send(ctx, Request{
		Provider: "test",
		URL:      "http://cancelled",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context cancellation", err)
	}
}

func TestResponseSummaryRedactsCredentialLikeBodies(t *testing.T) {
	got := responseSummary([]byte(`{"access_token":"secret"}`))
	const want = "[response body omitted because it may contain credentials]"
	if got != want {
		t.Fatalf("responseSummary() = %q, want %q", got, want)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

// bodyCut is a response whose body breaks before any byte arrives.
type bodyCut struct{ status int }

func (b bodyCut) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: b.status, Header: http.Header{},
		Body: io.NopCloser(errReader{}),
	}, nil
}

func sendThrough(rt http.RoundTripper) error {
	_, err := NewWithDependencies(Dependencies{
		HTTPClient: &http.Client{Transport: rt},
		Clock:      clock.RealClock{},
	}).Send(context.Background(),
		Request{Provider: "test", URL: "http://example.test/hook"})
	return err
}

func TestSendTreatsSuccessWithBrokenDrainAsDelivered(t *testing.T) {
	// The body is exactly the read limit, so only the drain after it hits
	// the broken reader.
	huge := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := io.MultiReader(
			io.LimitReader(zeroReader{}, 1<<20), errReader{})
		return &http.Response{StatusCode: 200, Header: http.Header{},
			Body: io.NopCloser(body)}, nil
	})

	if err := sendThrough(huge); err != nil {
		t.Fatalf("a 2xx whose drain broke must count as sent: %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestSendKeepsFailureStatusWithBrokenBody(t *testing.T) {
	err := sendThrough(bodyCut{status: 400})

	if code, ok := StatusOf(err); !ok || code != 400 {
		t.Fatalf("want the 400 status error, got %v", err)
	}
	if !IsPermanent(err) {
		t.Fatal("a 400 is permanent")
	}
}

func TestSendReportsSuccessWithUnreadableBody(t *testing.T) {
	if err := sendThrough(bodyCut{status: 200}); err == nil {
		t.Fatal("a success whose body never arrived cannot be verified")
	}
}

func TestSendRejectsBadHeaderAsPermanent(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"newline in value": {"X-Token": "abc\ndef"},
		"carriage return":  {"X-Token": "abc\rdef"},
		"space in name":    {"X Token": "abc"},
		"empty name":       {"": "abc"},
	} {
		t.Run(name, func(t *testing.T) {
			sender := NewWithDependencies(Dependencies{
				HTTPClient: &http.Client{Transport: bodyCut{status: 200}},
				Clock:      clock.RealClock{},
			})

			_, err := sender.Send(context.Background(), Request{
				Provider: "test", URL: "http://example.test/hook",
				Headers: headers,
			})

			if err == nil || !IsPermanent(err) {
				t.Fatalf("want a permanent error, got %v", err)
			}
			if strings.Contains(err.Error(), "def") {
				t.Fatalf("the header value leaked: %v", err)
			}
		})
	}
}
