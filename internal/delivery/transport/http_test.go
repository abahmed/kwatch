package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	_, err := New(http.DefaultClient).Send(context.Background(), Request{
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
	body, err := New(client).Send(context.Background(), Request{
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

	_, err := New(client).Send(ctx, Request{
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
