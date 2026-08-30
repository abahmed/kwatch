package util

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

var providerContexts = struct {
	sync.RWMutex
	values map[string]context.Context
}{values: make(map[string]context.Context)}

var providerContextLocks sync.Map

// WithProviderContext scopes a provider context to one delivery operation.
// The per-provider lock prevents two independent managers using the same
// provider name from borrowing one another's context.
func WithProviderContext(provider string, ctx context.Context, fn func()) {
	value, _ := providerContextLocks.LoadOrStore(provider, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	providerContexts.Lock()
	previous, hadPrevious := providerContexts.values[provider]
	if ctx == nil {
		ctx = context.Background()
	}
	providerContexts.values[provider] = ctx
	providerContexts.Unlock()
	defer func() {
		providerContexts.Lock()
		if hadPrevious {
			providerContexts.values[provider] = previous
		} else {
			delete(providerContexts.values, provider)
		}
		providerContexts.Unlock()
		mu.Unlock()
	}()
	fn()
}

func providerContext(provider string) context.Context {
	providerContexts.RLock()
	ctx := providerContexts.values[provider]
	providerContexts.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

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
	Context  context.Context
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
	ctx := r.Context
	if ctx == nil {
		ctx = providerContext(r.Provider)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.URL, bytes.NewReader(r.Body))
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

	const maxResponseBytes = 1 << 20
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))

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
	return PostContext(context.Background(), provider, url, body, contentType, headers)
}

// PostContext is the context-aware form of Post. Providers that can receive
// the delivery context should use this form so shutdown cancels an in-flight
// request instead of waiting for the transport timeout.
func PostContext(
	ctx context.Context,
	provider, url string,
	body []byte,
	contentType string,
	headers map[string]string,
) ([]byte, error) {
	return Send(
		Request{
			Provider:    provider,
			Context:     ctx,
			URL:         url,
			Body:        body,
			ContentType: contentType,
			Headers:     headers,
		},
	)
}
