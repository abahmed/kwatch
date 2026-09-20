// Package transport owns outbound provider HTTP behavior. Providers build
// payloads and request metadata; this package owns request execution and
// response classification.
package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// Request describes one provider HTTP request. Context and the HTTP client are
// supplied by Sender, keeping request data independent from runtime wiring.
type Request struct {
	Provider           string
	Method             string
	URL                string
	Body               []byte
	ContentType        string
	Headers            map[string]string
	BasicAuth          *BasicAuth
	RetryAfterFromBody func([]byte) time.Duration
}

// Sender executes provider HTTP requests with an explicit context and client.
// Providers depend on this small boundary instead of constructing requests.
type Sender interface {
	Send(context.Context, Request) ([]byte, error)
}

type client struct {
	httpClient *http.Client
	now        func() time.Time
}

// NewWithDependencies constructs a sender with all outbound dependencies
// fixed at provider construction time.
func NewWithDependencies(dependencies Dependencies) Sender {
	return &client{
		httpClient: dependencies.HTTPClient,
		now:        dependencies.Now,
	}
}

// BasicAuth carries HTTP basic-auth credentials.
type BasicAuth struct {
	Username string
	Password string
}

// ClassifyHTTPStatus applies the shared status policy to an SDK error.
func ClassifyHTTPStatus(status int, err error) error {
	if err == nil {
		return nil
	}
	if event.IsPermanentHTTPStatus(status) {
		return event.Permanent(err)
	}
	return err
}

// Send executes a provider request and returns its response body.
func (c *client) Send(ctx context.Context, r Request) ([]byte, error) {
	method := r.Method
	if method == "" {
		method = http.MethodPost
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(
		ctx, method, r.URL, bytes.NewReader(r.Body),
	)
	if err != nil {
		return nil, err
	}
	for key, value := range r.Headers {
		req.Header.Set(key, value)
	}
	if r.ContentType != "" {
		req.Header.Set("Content-Type", r.ContentType)
	} else if r.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.BasicAuth != nil {
		req.SetBasicAuth(r.BasicAuth.Username, r.BasicAuth.Password)
	}
	if c.httpClient == nil {
		return nil, fmt.Errorf("%s: outbound HTTP client is not configured",
			r.Provider)
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		// The request has already completed. Closing errors are not useful to
		// a provider retry decision, so they are intentionally ignored.
		_ = response.Body.Close()
	}()

	const maxResponseBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: read response body: %w", r.Provider, err)
	}
	// The bounded read protects memory use. Drain the remainder so the
	// application-owned HTTP transport can reuse the connection.
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return body, fmt.Errorf("%s: drain response body: %w", r.Provider, err)
	}
	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := ratelimit.ParseRetryAfterAt(response, c.now())
		if retryAfter == 0 && r.RetryAfterFromBody != nil {
			retryAfter = r.RetryAfterFromBody(body)
		}
		return body, &ratelimit.Error{
			Provider: r.Provider, StatusCode: response.StatusCode,
			RetryAfter: retryAfter,
		}
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		err := fmt.Errorf(
			"call to %s returned status code %d: %s",
			r.Provider, response.StatusCode, responseSummary(body),
		)
		if event.IsPermanentHTTPStatus(response.StatusCode) {
			return body, event.Permanent(err)
		}
		return body, err
	}
	return body, nil
}

func responseSummary(body []byte) string {
	const maxSummaryBytes = 512
	text := strings.TrimSpace(string(body))
	lower := strings.ToLower(text)
	for _, marker := range []string{
		"token", "secret", "password", "api_key", "apikey", "access_token",
	} {
		if strings.Contains(lower, marker) {
			return "[response body omitted because it may contain credentials]"
		}
	}
	if len(text) > maxSummaryBytes {
		cut := maxSummaryBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		return text[:cut] + "…"
	}
	return text
}
