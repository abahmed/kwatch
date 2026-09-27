package delivery

import (
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func TestRetryDelayCapsServerRetryAfter(t *testing.T) {
	rc := normalizeRetryConfig(retryConfig{maxAttempts: 3})
	for _, err := range []error{
		&ratelimit.Error{RetryAfter: 24 * time.Hour},
		&event.RetryAfterError{
			Err: errors.New("429"), RetryAfter: 2 * time.Hour,
		},
	} {
		delay, server := retryDelay(err, 1, rc)
		if !server || delay != maxServerRetryAfter {
			t.Fatalf("delay=%s server=%v, want capped wait", delay, server)
		}
	}
	delay, _ := retryDelay(&ratelimit.Error{RetryAfter: 5 * time.Second}, 1, rc)
	if delay != 5*time.Second {
		t.Fatalf("short Retry-After changed to %s", delay)
	}
}
