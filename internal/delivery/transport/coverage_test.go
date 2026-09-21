package transport

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/event"
)

func TestSendBuildsRequestWithHeadersAndBasicAuth(t *testing.T) {
	var got *http.Request
	client := &http.Client{Transport: roundTripFunc(func(
		req *http.Request,
	) (*http.Response, error) {
		got = req
		return response(http.StatusOK, "ok"), nil
	})}
	body, err := NewSender(Dependencies{
		HTTPClient: client, Clock: clock.RealClock{},
	}).Send(context.Background(), Request{
		Provider: "test", Method: http.MethodPut, URL: "http://test",
		Body: []byte("payload"), Headers: map[string]string{"X-Test": "yes"},
		BasicAuth: &BasicAuth{Username: "user", Password: "pass"},
	})
	if err != nil || string(body) != "ok" {
		t.Fatalf("Send() = %q, %v", body, err)
	}
	if got.Method != http.MethodPut || got.Header.Get("X-Test") != "yes" {
		t.Fatalf("request = %#v", got)
	}
	if got.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", got.Header.Get("Content-Type"))
	}
	user, pass, ok := got.BasicAuth()
	if !ok || user != "user" || pass != "pass" {
		t.Fatalf("basic auth = %q/%q/%t", user, pass, ok)
	}
}

func TestSendClassifiesErrorsAndUsesBodyRetryHint(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		want      string
		permanent bool
	}{
		{name: "client", status: http.StatusBadRequest, want: "bad",
			permanent: true},
		{name: "server", status: http.StatusBadGateway, want: "bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWithDependencies(Dependencies{
				HTTPClient: &http.Client{Transport: roundTripFunc(func(
					*http.Request,
				) (*http.Response, error) {
					return response(tt.status, tt.want), nil
				})}, Clock: clock.RealClock{},
			}).Send(context.Background(), Request{
				Provider: "test", URL: "http://test",
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Send() error = %v", err)
			}
			if tt.permanent && !event.IsPermanent(err) {
				t.Fatal("permanent error was not wrapped")
			}
		})
	}
	_, err := NewWithDependencies(Dependencies{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(
			*http.Request,
		) (*http.Response, error) {
			return response(http.StatusTooManyRequests, "busy"), nil
		})}, Clock: clock.Func(func() time.Time { return time.Unix(0, 0) }),
	}).Send(context.Background(), Request{
		Provider: "test", URL: "http://test",
		RetryAfterFromBody: func([]byte) time.Duration { return 3 * time.Second },
	})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("rate limit error = %v", err)
	}
}

func TestSendRejectsInvalidRequestsAndMissingClient(t *testing.T) {
	sender := NewWithDependencies(Dependencies{Clock: clock.RealClock{}})
	if _, err := sender.Send(context.Background(), Request{
		Provider: "test", URL: "http://test",
	}); err == nil {
		t.Fatal("missing client succeeded")
	}
	if _, err := sender.Send(context.Background(), Request{
		Provider: "test", URL: "://bad",
	}); err == nil {
		t.Fatal("invalid URL succeeded")
	}
}

func TestResponseSummaryTruncatesUTF8AndSafeText(t *testing.T) {
	text := strings.Repeat("界", 300)
	got := responseSummary([]byte(text))
	if !strings.HasSuffix(got, "…") || len(got) > 515 {
		t.Fatalf("summary length/suffix = %d/%q", len(got), got[len(got)-3:])
	}
	if got := responseSummary(
		[]byte(" plain response "),
	); got != "plain response" {
		t.Fatalf("safe summary = %q", got)
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
