package event

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

// RetryAfterError wraps an error with an optional Retry-After duration.
type RetryAfterError struct {
	Err        error
	RetryAfter time.Duration // 0 = use default backoff
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

// IsPermanent reports whether err, or anything it wraps, is a PermanentError.
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

// ClassifyHTTP marks err permanent when the HTTP status says the request
// itself was wrong, so providers that build their own status errors get the
// same retry behaviour as the ones using CheckHTTPResponse.
func ClassifyHTTP(code int, err error) error {
	if err != nil && IsPermanentHTTPStatus(code) {
		return Permanent(err)
	}
	return err
}

// CheckHTTPResponse returns an error for non-successful HTTP responses.
// For 429 status it returns a RetryAfterError that respects the Retry-After
// header.
// Other 4xx statuses return a PermanentError so they are not retried.
func CheckHTTPResponse(resp *http.Response, provider string) error {
	_, _ = io.Copy(
		io.Discard,
		resp.Body,
	) // best-effort drain; frees the conn on Close()
	if resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		d := time.Duration(0)
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				d = time.Duration(secs) * time.Second
			}
		}
		return &RetryAfterError{
			Err: fmt.Errorf(
				"call to %s returned status %d",
				provider,
				resp.StatusCode,
			),
			RetryAfter: d,
		}
	}
	err := fmt.Errorf(
		"call to %s returned status code %d",
		provider,
		resp.StatusCode,
	)
	if IsPermanentHTTPStatus(resp.StatusCode) {
		return Permanent(err)
	}
	return err
}

// Event used to represent info needed by providers to send messages
type Event struct {
	Resource     string // "pod", "node", "pvc"
	PodName      string
	PodUID       string // UID of the concrete Pod instance
	PodLineageID string // explicit stable lineage for ownerless Pods
	// PodGenerateName is evidence only; it is never an authoritative identity.
	PodGenerateName string
	ContainerName   string
	Image           string
	Message         string
	Namespace       string
	NodeName        string
	Reason          string
	Events          string
	Logs            string
	Labels          map[string]string
	OwnerKind       string
	RestartCount    int
	// Pre-computed diagnostic hint; empty = auto-generate from Reason
	Hint string
	// Structured details behind Hint; renderers read these, not the prose
	Facts model.Facts
	// Override severity; empty = let enricher decide from OwnerKind
	Severity      model.Severity
	IncludeEvents bool // If false, omit events section from output
	IncludeLogs   bool // If false, omit logs section from output
	// Action is the incident action: "create", "update", "resolved"; "" means
	// the legacy event path.
	Action string
	// Stable per-incident key for trigger↔resolve correlation
	DedupKey string
}
