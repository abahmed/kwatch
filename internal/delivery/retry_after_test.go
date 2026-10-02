package delivery

import (
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func TestServerRetryAfterCapsTheProviderWait(t *testing.T) {
	for _, err := range []error{
		&ratelimit.Error{RetryAfter: 24 * time.Hour},
		&transport.RetryAfterError{
			Err: errors.New("429"), RetryAfter: 2 * time.Hour,
		},
	} {
		wait, limited := serverRetryAfter(err)
		if !limited || wait != maxServerRetryAfter {
			t.Fatalf("wait=%s limited=%v, want capped wait", wait, limited)
		}
	}
	wait, _ := serverRetryAfter(&ratelimit.Error{RetryAfter: 5 * time.Second})
	if wait != 5*time.Second {
		t.Fatalf("short Retry-After changed to %s", wait)
	}
	if _, limited := serverRetryAfter(errors.New("timeout")); limited {
		t.Fatal("a plain error is not a rate limit")
	}
}
