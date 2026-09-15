package ratelimit

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

type Error struct {
	Provider   string
	StatusCode int
	RetryAfter time.Duration
	Err        error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s rate limited (status %d), retry after %s", e.Provider, e.StatusCode, e.RetryAfter)
}

func (e *Error) Unwrap() error { return e.Err }

func ParseRetryAfter(resp *http.Response) time.Duration {
	return ParseRetryAfterAt(resp, clock.RealClock{}.Now())
}

// ParseRetryAfterAt is the deterministic form used by retry logic and tests.
// HTTP-date values are relative to the supplied time rather than the process
// clock.
func ParseRetryAfterAt(resp *http.Response, now time.Time) time.Duration {
	if resp == nil {
		return 0
	}
	v := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
