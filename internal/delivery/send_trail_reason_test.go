package delivery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func statusErr(code int, body string) error {
	err := error(&transport.StatusError{Provider: "p", StatusCode: code,
		Body: body})
	if transport.IsPermanentHTTPStatus(code) {
		return transport.Permanent(err)
	}
	return err
}

func TestFailureReasonWordsEachFailure(t *testing.T) {
	dnsErr := &url.Error{Op: "Post", URL: "https://x",
		Err: &net.OpError{Op: "dial", Err: &net.DNSError{Name: "x"}}}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"success", nil, ""},
		{"too large status", statusErr(413, ""),
			"rejected: message too long"},
		{"too long body", statusErr(400, "text is too long"),
			"rejected: message too long"},
		{"server error mentioning a quota",
			statusErr(503, "quota exceeds the plan"),
			"server error (503), retrying"},
		{"exceeds the maximum", statusErr(400, "text exceeds the maximum"),
			"rejected: message too long"},
		{"plain exceeds is not a size", statusErr(400, "quota exceeds"),
			"rejected (400)"},
		{"slack code", transport.Permanent(errors.New("msg_too_long")),
			"rejected: message too long"},
		{"unauthorized", statusErr(401, "nope"), "token refused (401)"},
		{"forbidden", statusErr(403, ""), "access denied (403)"},
		{"not found", statusErr(404, ""), "not found (404)"},
		{"gone", statusErr(410, ""), "gone (410)"},
		{"other client error", statusErr(422, "bad field"),
			"rejected (422)"},
		{"rate limited", &ratelimit.Error{Provider: "p", StatusCode: 429},
			"rate limited, retrying"},
		{"server error", statusErr(502, ""),
			"server error (502), retrying"},
		{"deadline", fmt.Errorf("send: %w", context.DeadlineExceeded),
			"timed out"},
		{"network", dnsErr, "network error"},
		{"slack auth code", transport.Permanent(errors.New("invalid_auth")),
			"token refused"},
		{"slack channel code",
			transport.Permanent(errors.New("channel_not_found")),
			"not found"},
		{"redirect", transport.Permanent(errors.New(
			"call to p was redirected (status code 302)")),
			"redirected: check the URL"},
		{"other permanent", transport.Permanent(errors.New("nope")),
			"rejected"},
		{"metadata address", &url.Error{Op: "Post", URL: "https://x",
			Err: transport.Permanent(transport.ErrMetadataAddress)},
			"blocked: cloud metadata address"},
		{"metadata address at connect", &url.Error{Op: "Post",
			URL: "https://x", Err: &net.OpError{Op: "dial",
				Err: transport.Permanent(transport.ErrMetadataAddress)}},
			"blocked: cloud metadata address"},
		{"other transient", errors.New("boom"), "failed, retrying"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := failureReason(tc.err); got != tc.want {
				t.Fatalf("failureReason = %q, want %q", got, tc.want)
			}
		})
	}
}
