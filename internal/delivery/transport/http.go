// Package transport owns outbound provider HTTP behavior. Providers build
// payloads and request metadata; this package owns request execution and
// response classification.
package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

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
	if IsPermanentHTTPStatus(status) {
		return Permanent(err)
	}
	return err
}

// Send executes a provider request and returns its response body.
func (c *client) Send(ctx context.Context, r Request) ([]byte, error) {
	req, err := buildRequest(ctx, r)
	if err != nil {
		return nil, err
	}
	if c.httpClient == nil {
		return nil, fmt.Errorf("%s: outbound HTTP client is not configured",
			r.Provider)
	}
	response, err := noRedirects(c.httpClient).Do(req)
	if err != nil {
		return nil, RedactURLError(err)
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
	// The drain is capped too: a hostile endpoint must not hold the worker.
	const maxDrainBytes = 1 << 20
	if _, err := io.CopyN(io.Discard, response.Body, maxDrainBytes); err != nil &&
		!errors.Is(err, io.EOF) {
		return body, fmt.Errorf("%s: drain response body: %w", r.Provider, err)
	}
	return classifyResponse(r, response, body, c.now)
}

// noRedirects returns a copy of c that never follows a redirect: a 301, 302
// or 303 would turn the POST into a GET that is counted as delivered, and a
// redirect to another host would carry the provider's auth headers there.
func noRedirects(c *http.Client) *http.Client {
	copied := *c
	copied.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copied
}

// buildRequest turns a provider Request into an HTTP request with its
// headers and credentials. A URL that cannot be parsed is permanent.
func buildRequest(ctx context.Context, r Request) (*http.Request, error) {
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
		// A URL that cannot be parsed never becomes valid on retry, and the
		// parse error quotes the URL, which may carry a token.
		return nil, Permanent(fmt.Errorf("%s: build request: %w",
			r.Provider, RedactURLError(err)))
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
	return req, nil
}

// classifyResponse maps the status of a completed response to success, a
// rate-limit error, a permanent error or a retryable error.
func classifyResponse(
	r Request, response *http.Response, body []byte,
	now func() time.Time,
) ([]byte, error) {
	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := ratelimit.ParseRetryAfterAt(response, now())
		if retryAfter == 0 && r.RetryAfterFromBody != nil {
			retryAfter = r.RetryAfterFromBody(body)
		}
		return body, &ratelimit.Error{
			Provider: r.Provider, StatusCode: response.StatusCode,
			RetryAfter: retryAfter,
		}
	}
	if response.StatusCode >= 300 && response.StatusCode <= 399 {
		return body, Permanent(fmt.Errorf(
			"call to %s was redirected (status code %d) and kwatch does "+
				"not follow redirects: check the configured URL",
			r.Provider, response.StatusCode))
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return body, failureOf(r, response, body, now)
	}
	return body, nil
}

// failureOf is the error of a non-2xx answer: a StatusError, permanent for
// a client error, with the wait of a 503 that names one.
func failureOf(
	r Request, response *http.Response, body []byte,
	now func() time.Time,
) error {
	var err error = &StatusError{
		Provider: r.Provider, StatusCode: response.StatusCode,
		Body: responseSummary(body),
	}
	switch {
	case IsPermanentHTTPStatus(response.StatusCode):
		return Permanent(err)
	case response.StatusCode == http.StatusServiceUnavailable:
		// A busy server says how long to wait; honour it like a rate
		// limit.
		if wait := ratelimit.ParseRetryAfterAt(response, now()); wait > 0 {
			return &RetryAfterError{Err: err, RetryAfter: wait}
		}
	}
	return err
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

// RedactURLError removes the request URL from client errors. Provider URLs
// often carry credentials in the path or query (bot tokens, webhook secrets,
// integration keys), and callers log these errors.
func RedactURLError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	redacted := *urlErr
	redacted.URL = redactedURL(urlErr.URL)
	return &redacted
}

// redactedURL keeps only the scheme and host of a request URL.
func redactedURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "[redacted]"
	}
	return parsed.Scheme + "://" + parsed.Host + "/[redacted]"
}

// LogURL is the form of a configured provider URL that is safe to log: the
// scheme and host only. Credentials in the user info, path or query never
// reach the logs. A URL without a host logs as "[invalid]".
func LogURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "[invalid]"
	}
	return parsed.Scheme + "://" + parsed.Host
}
