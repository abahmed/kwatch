package notification

import (
	"fmt"
	"net/url"
	"strings"
)

// LinkSpan is a span that shows label and links to address.
func LinkSpan(label, address string) Span {
	return Span{Text: label, Style: Link, URL: address}
}

// plain is the span's text as plain words. A link is its label and its
// address, "label: address", the way a line without markup reads; a
// label that already ends in a colon is followed by the address alone.
func (s Span) plain() string {
	if s.Style != Link || s.URL == "" {
		return s.Text
	}
	return linkWords(s.Text, s.URL)
}

// linkWords writes a link as plain words.
func linkWords(label, address string) string {
	label = strings.TrimSpace(label)
	switch {
	case label == "":
		return address
	case strings.HasSuffix(label, ":"):
		return label + " " + address
	default:
		return label + ": " + address
	}
}

// renderLink writes a link span. Only an http or https address becomes
// markup; any other address, a dialect without links and an empty label
// all write the plain words, escaped as text.
func renderLink(s Span, d Dialect) string {
	address, ok := linkURL(s.URL)
	if !ok || d.Link == nil || strings.TrimSpace(s.Text) == "" {
		return d.Text(d.neutral(s.plain()))
	}
	return d.Link(d.neutral(s.Text), address)
}

// linkURL checks that raw is an absolute http or https address and
// percent-encodes, after the host, every byte that could end or break
// the markup around it: spaces and controls, quotes, brackets, parens,
// angle brackets, the pipe, the backslash and non-ASCII bytes. A host
// that holds such a byte is refused.
func linkURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	start := len(u.Scheme) + len("://")
	if !strings.HasPrefix(raw[len(u.Scheme):], "://") {
		return "", false
	}
	end := start + strings.IndexAny(raw[start:]+"/", "/?#")
	if strings.ContainsFunc(raw[start:end], badHostRune) {
		return "", false
	}
	return raw[:end] + encodeUnsafe(raw[end:]), true
}

// badHostRune is a character a host must not hold in a link: one that
// would end the markup. Brackets stay, for an IPv6 host.
func badHostRune(r rune) bool {
	return r <= ' ' || r >= 0x7f ||
		strings.ContainsRune(`"'<>()|\{}^`+"`", r)
}

// unsafeLinkBytes are the bytes, besides spaces, controls and non-ASCII,
// that a link's path, query or fragment holds only percent-encoded.
const unsafeLinkBytes = `"'<>()[]{}|\^` + "`"

// encodeUnsafe percent-encodes the unsafe bytes of s, leaving every
// other byte, an existing escape included, as it is.
func encodeUnsafe(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= ' ' || c >= 0x7f ||
			strings.IndexByte(unsafeLinkBytes, c) >= 0 {
			fmt.Fprintf(&sb, "%%%02X", c)
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}
