package delivery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

// tooLongHints are the words providers use when they refuse a message
// for its size: an HTTP body or a bare SDK code such as msg_too_long.
var tooLongHints = []string{"too long", "too_long", "too large",
	"too_large", "too big", "exceeds", "max length", "maximum length"}

// tokenHints are the bare SDK codes of a refused credential.
var tokenHints = []string{"invalid_auth", "not_authed", "token_revoked",
	"token_expired", "account_inactive", "unauthorized"}

// failureReason is a short plain-language reason a send failed, for the
// dashboard: "rejected: message too long", "token refused (401)" and so
// on. It is "" for a send that worked. It never quotes the error, which
// the trail keeps scrubbed in Error.
func failureReason(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	var limited *ratelimit.Error
	switch {
	case errors.Is(err, transport.ErrMetadataAddress):
		return "blocked: cloud metadata address"
	case errors.As(err, &limited):
		return "rate limited, retrying"
	case hasAny(text, tooLongHints):
		return "rejected: message too long"
	case isTimeout(err):
		return "timed out"
	}
	if code, ok := transport.StatusOf(err); ok {
		return statusReason(code)
	}
	switch {
	case strings.Contains(text, "redirected"):
		return "redirected: check the URL"
	case hasAny(text, tokenHints):
		return "token refused"
	case strings.Contains(text, "not_found"):
		return "not found"
	case isNetworkError(err):
		return "network error"
	case transport.IsPermanent(err):
		return "rejected"
	}
	return "failed, retrying"
}

// statusReason words an HTTP status the provider answered.
func statusReason(code int) string {
	switch {
	case code == http.StatusRequestEntityTooLarge:
		return "rejected: message too long"
	case code == http.StatusUnauthorized:
		return "token refused (401)"
	case code == http.StatusForbidden:
		return "access denied (403)"
	case code == http.StatusNotFound:
		return "not found (404)"
	case code == http.StatusGone:
		return "gone (410)"
	case code == http.StatusTooManyRequests:
		return "rate limited, retrying"
	case code >= 500:
		return fmt.Sprintf("server error (%d), retrying", code)
	case code >= 400:
		return fmt.Sprintf("rejected (%d)", code)
	}
	return fmt.Sprintf("unexpected answer (%d)", code)
}

func hasAny(text string, words []string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isNetworkError(err error) bool {
	var netErr net.Error
	var opErr *net.OpError
	var dnsErr *net.DNSError
	return errors.As(err, &netErr) || errors.As(err, &opErr) ||
		errors.As(err, &dnsErr)
}
