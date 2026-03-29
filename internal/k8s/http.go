package k8s

import (
	"net/http"
	"time"
)

const (
	DefaultHTTPTimeout = 30 * time.Second
)

var defaultClient *http.Client

func init() {
	defaultClient = &http.Client{Timeout: DefaultHTTPTimeout}
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout == 0 {
		timeout = DefaultHTTPTimeout
	}
	return &http.Client{Timeout: timeout}
}

func GetDefaultClient() *http.Client {
	return defaultClient
}
