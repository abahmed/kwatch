package dashboard

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"testing"
)

// pageHeaderLines are the security headers every dashboard page sends,
// as they read on the wire.
var pageHeaderLines = []string{
	"X-Content-Type-Options: nosniff\r\n",
	"X-Frame-Options: DENY\r\n",
	"Cache-Control: no-store\r\n",
	"Referrer-Policy: no-referrer\r\n",
	"Content-Security-Policy: " + contentSecurityPolicy + "\r\n",
	"Cross-Origin-Opener-Policy: same-origin\r\n",
	"Cross-Origin-Resource-Policy: same-origin\r\n",
}

// The 400 for an unparsable request carries the security headers of
// every other page, without HSTS over plain HTTP.
func TestPageListenerBadRequestCarriesThePageHeaders(t *testing.T) {
	got := rawGet(t, "/issues/%E0%A4%A")
	for _, line := range pageHeaderLines {
		if !strings.Contains(got, line) {
			t.Errorf("reply lacks %q: %q", line, got)
		}
	}
	if strings.Contains(got, "Strict-Transport-Security") {
		t.Errorf("plain HTTP reply sends HSTS: %q", got)
	}
}

// tlsBrokenEscape sends a request with a broken escape over TLS to
// addr and returns the whole reply.
func tlsBrokenEscape(t *testing.T, addr string) string {
	t.Helper()
	c, err := tlsDial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = fmt.Fprintf(c, "GET /issues/%%E0%%A4%%A HTTP/1.1\r\nHost: x\r\n"+
		"Connection: close\r\n\r\n")
	body, _ := io.ReadAll(bufio.NewReader(c))
	return string(body)
}

// In shared mode over kwatch's own TLS every page sends HSTS, and so
// does the 400; outside shared mode neither does.
func TestTLSServeBadRequestSendsHSTSInSharedMode(t *testing.T) {
	hsts := "Strict-Transport-Security: " + hstsValue + "\r\n"
	addr, _ := serveTLSMode(t, true)
	got := tlsBrokenEscape(t, addr)
	if !strings.HasPrefix(got, "HTTP/1.1 400 ") ||
		!strings.Contains(got, hsts) {
		t.Errorf("shared reply = %q", got)
	}
	for _, line := range pageHeaderLines {
		if !strings.Contains(got, line) {
			t.Errorf("shared reply lacks %q", line)
		}
	}
	addr, _ = serveTLS(t)
	if got := tlsBrokenEscape(t, addr); strings.Contains(got, hsts) {
		t.Errorf("unshared reply sends HSTS: %q", got)
	}
}
