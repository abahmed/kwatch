package util

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// Request is one call to a provider's HTTP API.
//
// Every provider that talks HTTP goes through Send. That is what makes status
// handling uniform: any 2xx is success, 429 becomes a *ratelimit.Error the
// delivery retry honours, other 4xx are permanent (a bad payload will not get
// better on retry), 5xx and transport errors are retried. Before this, a dozen
// providers each had their own copy of that logic — and their own bugs in it.
type Request struct {
	// Provider names the integration in errors and rate-limit reports.
	Provider string
	// Method defaults to POST.
	Method string
	URL    string
	Body   []byte
	// ContentType defaults to application/json when a body is sent and no
	// Content-Type header is given.
	ContentType string
	Headers     map[string]string
	BasicAuth   *BasicAuth
	// RetryAfterFromBody extracts a rate-limit delay from a 429 body for APIs
	// that put it there instead of in the Retry-After header (Telegram).
	// Consulted only when the header is absent.
	RetryAfterFromBody func(body []byte) time.Duration
}

// BasicAuth carries HTTP basic-auth credentials.
type BasicAuth struct {
	Username string
	Password string
}

// Send performs the request and returns the response body. See Request for
// how each status class is classified.
func Send(r Request) ([]byte, error) {
	method := r.Method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequest(method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	if r.ContentType != "" {
		req.Header.Set("Content-Type", r.ContentType)
	} else if r.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.BasicAuth != nil {
		req.SetBasicAuth(r.BasicAuth.Username, r.BasicAuth.Password)
	}

	resp, err := k8s.GetDefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := ratelimit.ParseRetryAfter(resp)
		if retryAfter == 0 && r.RetryAfterFromBody != nil {
			retryAfter = r.RetryAfterFromBody(respBody)
		}
		return respBody, &ratelimit.Error{
			Provider:   r.Provider,
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: retryAfter,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err := fmt.Errorf(
			"call to %s returned status code %d: %s",
			r.Provider, resp.StatusCode, strings.TrimSpace(string(respBody)))
		// A 4xx means the request is wrong, not that the server is busy.
		// Retrying a bad payload three times just delays the alerts behind it.
		if event.IsPermanentHTTPStatus(resp.StatusCode) {
			return respBody, event.Permanent(err)
		}
		return respBody, err
	}

	return respBody, nil
}

// Post sends an HTTP POST to url with the given body, content type and extra
// headers, returning the response body on success. It is Send for the common
// case; see Request for the status handling.
func Post(
	provider, url string,
	body []byte,
	contentType string,
	headers map[string]string,
) ([]byte, error) {
	return Send(
		Request{
			Provider:    provider,
			URL:         url,
			Body:        body,
			ContentType: contentType,
			Headers:     headers,
		},
	)
}
