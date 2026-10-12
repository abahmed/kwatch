package slack

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"

	slackClient "github.com/slack-go/slack"
)

func TestSlackUnknownAPIErrorIsPermanent(t *testing.T) {
	for _, code := range []string{
		"restricted_action", "access_denied", "no_permission",
		"user_is_restricted", "team_access_not_granted",
		"ekm_access_denied", "missing_scope", "channel_is_archived",
		"some_future_code",
	} {
		t.Run(code, func(t *testing.T) {
			err := wrapSlackRateLimit(errors.New(code))
			if !transport.IsPermanent(err) {
				t.Fatalf("%s should be permanent: %v", code, err)
			}
		})
	}
}

func TestSlackTransientAPIErrorIsRetryable(t *testing.T) {
	for code := range transientSlackErrors {
		t.Run(code, func(t *testing.T) {
			err := wrapSlackRateLimit(errors.New(code))
			if transport.IsPermanent(err) {
				t.Fatalf("%s should be retryable: %v", code, err)
			}
		})
	}
}

func TestSlackNonAPIErrorKeepsClassification(t *testing.T) {
	for _, err := range []error{
		context.Canceled,
		errors.New("dial tcp 10.0.0.1:443: connection refused"),
		errors.New("EOF"),
	} {
		got := wrapSlackRateLimit(err)
		if transport.IsPermanent(got) {
			t.Fatalf("%v should stay retryable", err)
		}
	}
}

func TestSlackRateLimitedErrorStaysRateLimited(t *testing.T) {
	err := wrapSlackRateLimit(
		&slackClient.RateLimitedError{RetryAfter: 3 * time.Second})
	var rle *ratelimit.Error
	if !errors.As(err, &rle) || rle.RetryAfter != 3*time.Second {
		t.Fatalf("want rate limit error, got %v", err)
	}
	err = wrapSlackRateLimit(slackClient.StatusCodeError{
		Code: http.StatusTooManyRequests,
	})
	if !errors.As(err, &rle) {
		t.Fatalf("429 should be rate limited: %v", err)
	}
}

func TestSlackStaleThreadErrorStaysDetectable(t *testing.T) {
	err := wrapSlackRateLimit(errors.New("thread_not_found"))
	if !isStaleThreadError(err) {
		t.Fatalf("stale thread lost after wrapping: %v", err)
	}
}
