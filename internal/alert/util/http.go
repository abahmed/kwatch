package util

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// Post sends an HTTP POST to url with the given body, content type and extra
// headers, returning the response body on success. Any 2xx status is treated
// as success. 429 responses return a *ratelimit.Error honoring the Retry-After
// header; any other non-2xx status returns a descriptive error.
// The provider name is used in error messages and ratelimit reporting.
func Post(provider, url string, body []byte, contentType string, headers map[string]string) ([]byte, error) {
	client := k8s.GetDefaultClient()
	buffer := bytes.NewBuffer(body)
	req, err := http.NewRequest(http.MethodPost, url, buffer)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return respBody, &ratelimit.Error{
			Provider:   provider,
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: ratelimit.ParseRetryAfter(resp),
		}
	}
	if resp.StatusCode > 299 {
		return respBody, fmt.Errorf(
			"call to %s returned status code %d: %s",
			provider, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return respBody, nil
}
