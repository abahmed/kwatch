package redact

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/metrics"
)

// nextPair matches what follows a "," or ";" that ends a bare value: the
// next "key=" pair of a connection string, or a quote that closes it.
var nextPair = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_.]*=|["'])`)

// nextParameter matches what follows an "&" that ends a bare value: the
// next "key=" parameter of a query string or a form body.
var nextParameter = regexp.MustCompile(`^&[A-Za-z_][A-Za-z0-9_.\[\]-]*=`)

// A bare value ends at the first space, but a raw password may hold
// "&", ";", "," and quotes: "p&ss=word1" or "ab;c=part" is one
// password. Only where the pair sits in a list of pairs is such a
// character the end of the value: an "&" in a query string or form
// (the key follows "?" or "&"), a ";" or "," in a connection string
// (the key follows ";" or ","). Anywhere else the value is redacted to
// the end of its word: hiding a little more beats showing a secret.

// pairList is the list the pair at start sits in: the character before
// its key, or the one its capture begins with; 0 for none.
func pairList(text string, start int, key string) byte {
	if key != "" && strings.IndexByte("?&;,", key[0]) >= 0 {
		return key[0]
	}
	if start > 0 && strings.IndexByte("?&;,", text[start-1]) >= 0 {
		return text[start-1]
	}
	return 0
}

// splitSecret cuts a bare value where it ends: at a "," or ";" that is
// at the end of the value, in a connection string (list ";" or ",")
// also before the next pair or closing quote, in a query string (list
// "?" or "&") at an "&" before the next parameter, and at the quote
// that closes the text the pair is quoted in.
func splitSecret(value string, list byte) (secret, tail string) {
	pairs := list == ';' || list == ','
	query := list == '?' || list == '&'
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case ',', ';':
			rest := strings.TrimLeft(value[i:], ",;")
			if rest == "" || pairs && nextPair.MatchString(rest) {
				return value[:i], value[i:]
			}
		case '&':
			if query && nextParameter.MatchString(value[i:]) {
				return value[:i], value[i:]
			}
		}
	}
	return splitClosingQuote(value)
}

// splitClosingQuote gives back the quote a bare value ends with. The
// value did not open it, so it closes text around the whole pair, as in
// curl -H 'X-Api-Key: abcd': without it the reader sees a quote that
// never ends.
func splitClosingQuote(value string) (secret, tail string) {
	end := len(strings.TrimRight(value, `"'`))
	if end == 0 || strings.ContainsAny(value[:end], `"'`) {
		return value, ""
	}
	return value[:end], value[end:]
}

// rebuildKeyValueAt keeps the key and its separator, replaces the value
// and keeps what follows a bare value (see splitSecret). groups[1] is
// the key with its separator and groups[2] the value; start is where
// the match begins in text.
func rebuildKeyValueAt(text string, start int, groups []string) string {
	value := groups[2]
	if value == "" || value[0] == '"' || value[0] == '\'' {
		return groups[1] + redacted
	}
	_, tail := splitSecret(value, pairList(text, start, groups[1]))
	return groups[1] + redacted + tail
}

// applyInContext is applyRedaction for a rule with rebuildAt.
func applyInContext(value string, rule redactionRule) string {
	var b strings.Builder
	last := 0
	for _, loc := range rule.pattern.FindAllStringSubmatchIndex(value, -1) {
		groups := make([]string, len(loc)/2)
		for i := range groups {
			if loc[2*i] >= 0 {
				groups[i] = value[loc[2*i]:loc[2*i+1]]
			}
		}
		b.WriteString(value[last:loc[0]])
		last = loc[1]
		if rule.skip != nil && rule.skip(groups) {
			b.WriteString(groups[0])
			continue
		}
		metrics.DefaultRegistry().RedactedValues.Add(1)
		b.WriteString(rule.rebuildAt(value, loc[0], groups))
	}
	b.WriteString(value[last:])
	return b.String()
}
