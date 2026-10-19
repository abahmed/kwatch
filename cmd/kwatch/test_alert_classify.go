package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/redact"
)

// testFailure is the detail and fix line printed for a failed provider.
type testFailure struct{ detail, fix string }

// classifyTestFailure maps a send error to the spec's wording. Details
// never carry a URL, token, user or address: the fallback text goes
// through the diagnostics redaction list.
func classifyTestFailure(err error, timeout string) testFailure {
	if f, ok := classifyStatus(err); ok {
		return f
	}
	var smtpErr *textproto.Error
	if errors.As(err, &smtpErr) && smtpErr.Code == 535 {
		return testFailure{
			"the mail server refused the login (535)",
			"Fix: check username and password under Edit " +
				"notification providers.",
		}
	}
	if isTLSFailure(err) {
		return testFailure{"certificate not trusted",
			"Fix: a proxy or custom certificate is in the way. " +
				"See docs.kwatch.dev."}
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) ||
		(errors.As(err, &netErr) && netErr.Timeout()) {
		return testFailure{"no answer after " + timeout,
			"Fix: check egress rules; the provider may be slow."}
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || errors.As(err, &opErr) {
		return testFailure{"could not reach the provider",
			"Fix: the pod cannot reach the internet. Check egress " +
				"rules (network policy) and DNS."}
	}
	return testFailure{detail: redact.Diagnostics(err.Error())}
}

func classifyStatus(err error) (testFailure, bool) {
	code, ok := transport.StatusOf(err)
	if !ok {
		return testFailure{}, false
	}
	switch {
	case code == 401 || code == 403:
		return testFailure{
			fmt.Sprintf("rejected the credentials (%d)", code),
			"Fix: the token or key is wrong or expired. Update it " +
				"in Edit notification providers.",
		}, true
	case code == 404:
		return testFailure{"the webhook returned 404 (not found)",
			"Fix: the webhook was deleted or mistyped. Create a new " +
				"one, then Edit notification providers."}, true
	case code == 429:
		return testFailure{"rate limited (429)",
			"Fix: wait a minute and run the test again."}, true
	case code >= 500:
		return testFailure{
			fmt.Sprintf("the provider returned %d", code),
			"Fix: this is on their side. Try again later.",
		}, true
	}
	return testFailure{
		detail: fmt.Sprintf("the provider returned %d", code),
	}, true
}

func isTLSFailure(err error) bool {
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	var verify *tls.CertificateVerificationError
	return errors.As(err, &unknown) || errors.As(err, &invalid) ||
		errors.As(err, &hostname) || errors.As(err, &verify) ||
		strings.Contains(err.Error(), "x509:")
}
