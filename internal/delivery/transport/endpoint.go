package transport

import (
	"net/url"
	"strings"
)

// ValidEndpoint reports whether raw is an absolute http or https URL with
// a host. Provider constructors use it to reject a mistyped webhook or
// server URL at startup, so a broken setting fails lint and startup with
// a clear message instead of failing every delivery later.
func ValidEndpoint(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http"
}
