package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func sendTo(t *testing.T, handler http.HandlerFunc) error {
	t.Helper()
	server := httptest.NewServer(handler)
	defer server.Close()
	_, err := NewWithDependencies(Dependencies{
		HTTPClient: http.DefaultClient, Clock: clock.RealClock{},
	}).Send(context.Background(), Request{Provider: "test", URL: server.URL})
	return err
}

func replyWith(code int, retryAfter string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte("nope"))
	}
}

func TestSendReturnsTypedStatusError(t *testing.T) {
	err := sendTo(t, replyWith(http.StatusNotFound, ""))
	var status *StatusError
	if !errors.As(err, &status) {
		t.Fatalf("error = %T, want a *StatusError inside", err)
	}
	if status.Provider != "test" || status.StatusCode != 404 ||
		status.Body != "nope" {
		t.Fatalf("status error = %+v", status)
	}
	want := "call to test returned status code 404: nope"
	if err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err.Error(), want)
	}
	if !IsPermanent(err) {
		t.Fatal("a 404 must stay permanent")
	}
}

func TestStatusOfAndIsNotFound(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		code     int
		ok       bool
		notFound bool
	}{
		{"nil", nil, 0, false, false},
		{"plain error", errors.New("status code 404"), 0, false, false},
		{"404", &StatusError{StatusCode: 404}, 404, true, true},
		{"wrapped 404", fmt.Errorf("x: %w",
			Permanent(&StatusError{StatusCode: 404})), 404, true, true},
		{"500 saying 404", &StatusError{StatusCode: 500,
			Body: "status code 404"}, 500, true, false},
		{"rate limit", &ratelimit.Error{StatusCode: 429}, 429, true, false},
	}
	for _, c := range cases {
		code, ok := StatusOf(c.err)
		if code != c.code || ok != c.ok {
			t.Errorf("%s: StatusOf = %d, %v", c.name, code, ok)
		}
		if IsNotFound(c.err) != c.notFound {
			t.Errorf("%s: IsNotFound = %v", c.name, !c.notFound)
		}
	}
}

func TestServiceUnavailableHonoursRetryAfter(t *testing.T) {
	err := sendTo(t, replyWith(http.StatusServiceUnavailable, "9"))
	var wait *RetryAfterError
	if !errors.As(err, &wait) || wait.RetryAfter != 9*time.Second {
		t.Fatalf("error = %v, want a 9s RetryAfterError", err)
	}
	if code, _ := StatusOf(err); code != 503 {
		t.Fatalf("StatusOf = %d, want 503", code)
	}
}

func TestServerErrorWithoutRetryAfterIsPlain(t *testing.T) {
	err := sendTo(t, replyWith(http.StatusBadGateway, ""))
	var wait *RetryAfterError
	if errors.As(err, &wait) {
		t.Fatalf("502 without the header must not carry a wait: %v", err)
	}
}
