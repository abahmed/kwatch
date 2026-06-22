package event

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// RetryAfterError wraps an error with an optional Retry-After duration.
type RetryAfterError struct {
	Err        error
	RetryAfter time.Duration // 0 = use default backoff
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// CheckHTTPResponse returns an error for non-successful HTTP responses.
// For 429 status it returns a RetryAfterError that respects the Retry-After header.
func CheckHTTPResponse(resp *http.Response, provider string) error {
	defer io.Copy(io.Discard, resp.Body) // drain so the caller's deferred Close() returns the conn to the pool
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
			Err:        fmt.Errorf("call to %s returned status %d", provider, resp.StatusCode),
			RetryAfter: d,
		}
	}
	return fmt.Errorf("call to %s returned status code %d", provider, resp.StatusCode)
}

// Event used to represent info needed by providers to send messages
type Event struct {
	Resource      string // "pod", "node", "pvc"
	PodName       string
	ContainerName string
	Namespace     string
	NodeName      string
	Reason        string
	Events        string
	Logs          string
	Labels        map[string]string
	OwnerKind     string
	RestartCount  int
	Hint          string // Pre-computed diagnostic hint; empty = auto-generate from Reason
	Severity      string // Override severity; empty = let enricher decide from OwnerKind
	IncludeEvents bool   // If false, omit events section from output
	IncludeLogs   bool   // If false, omit logs section from output
	Action        string // Incident action: "create", "update", "resolved"; "" = legacy event path
	DedupKey      string // Stable per-incident key for trigger↔resolve correlation
}
