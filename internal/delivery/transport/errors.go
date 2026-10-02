package transport

import (
	"errors"
	"net/http"
	"time"
)

// RetryAfterError wraps an error with the delay the provider asked for.
// A zero RetryAfter means "use the default backoff".
type RetryAfterError struct {
	Err        error
	RetryAfter time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// PermanentError marks a delivery failure that retrying cannot fix: a
// malformed payload, a revoked token, an unknown channel. Retrying it only
// delays every alert queued behind it on the same provider.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent wraps err so retry logic gives up immediately.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{Err: err}
}

// IsPermanent reports whether err, or anything it wraps, is permanent.
func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}

// IsPermanentHTTPStatus reports whether an HTTP status means the request
// itself is wrong rather than the server being briefly unavailable. Client
// errors are permanent except 408 (request timeout) and 429 (rate limited),
// which explicitly ask for another attempt.
func IsPermanentHTTPStatus(code int) bool {
	if code < 400 || code >= 500 {
		return false
	}
	return code != http.StatusRequestTimeout &&
		code != http.StatusTooManyRequests
}
