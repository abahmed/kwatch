package providertest

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const redacted = "<redacted>"

// longString is the rune count above which a snapshot shortens a string
// to its head, tail, length and hash. The hash keeps the golden exact.
const longString = 400

var (
	isoTime = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?` +
		`(Z|[+-]\d\d:\d\d)`)
	sensitive = []string{"token", "key", "signature", "secret", "auth",
		"timestamp", "date", "nonce", "sign"}
)

// SnapshotOptions tune how a request is rendered.
type SnapshotOptions struct {
	// Headers are the header names to show besides Content-Type.
	Headers []string
	// Secrets are literal values replaced by <secret> everywhere.
	Secrets []string
}

// Snapshot renders one request as a stable, redacted, normalized text:
// method, path, sorted query, chosen headers, then the normalized body.
func Snapshot(r Request, o SnapshotOptions) string {
	var b strings.Builder
	path := r.RawPath
	if path == "" {
		path = r.Path
	}
	fmt.Fprintf(&b, "%s %s", r.Method, path)
	if q := normalizeQuery(r.Query); q != "" {
		b.WriteString("?" + q)
	}
	b.WriteString("\n")
	names := append([]string{"Content-Type"}, o.Headers...)
	for _, name := range names {
		if v := r.Header.Get(name); v != "" {
			fmt.Fprintf(&b, "%s: %s\n", name, redactHeader(name, v))
		}
	}
	b.WriteString("\n")
	b.WriteString(NormalizeBody(r.Header.Get("Content-Type"), r.Body))
	out := b.String()
	for _, s := range o.Secrets {
		if s != "" {
			out = strings.ReplaceAll(out, s, "<secret>")
		}
	}
	return isoTime.ReplaceAllString(out, "<time>")
}

func redactHeader(name, value string) string {
	if strings.EqualFold(name, "Authorization") {
		scheme, _, ok := strings.Cut(value, " ")
		if !ok {
			return redacted
		}
		return scheme + " " + redacted
	}
	lower := strings.ToLower(name)
	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return redacted
		}
	}
	return value
}

func normalizeQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		for _, v := range values[k] {
			if isSensitiveName(k) {
				v = redacted
			}
			parts = append(parts, k+"="+shorten(v))
		}
	}
	return strings.Join(parts, "&")
}

func isSensitiveName(name string) bool {
	lower := strings.ToLower(name)
	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// NormalizeBody renders JSON with sorted keys and indentation, forms as
// sorted decoded lines, and anything else as shortened text.
func NormalizeBody(contentType string, body []byte) string {
	if len(body) == 0 {
		return "(empty body)\n"
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		if out, ok := normalizeJSON(trimmed); ok {
			return out
		}
	}
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		if q := normalizeQuery(string(body)); q != "" {
			return strings.ReplaceAll(q, "&", "\n") + "\n"
		}
	}
	return shorten(string(body)) + "\n"
}

func normalizeJSON(body []byte) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", false
	}
	v = shortenTree(v)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", false
	}
	return buf.String(), true
}

func shortenTree(v any) any {
	switch x := v.(type) {
	case string:
		return shorten(x)
	case []any:
		for i := range x {
			x[i] = shortenTree(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = shortenTree(x[k])
		}
	}
	return v
}

// shorten keeps short strings and reduces long ones to head, tail, rune
// count and a hash of the whole.
func shorten(s string) string {
	n := utf8.RuneCountInString(s)
	if n <= longString {
		return s
	}
	r := []rune(s)
	return fmt.Sprintf("%s…[%d runes sha256:%.12x]…%s",
		string(r[:100]), n, sha256.Sum256([]byte(s)), string(r[n-100:]))
}
