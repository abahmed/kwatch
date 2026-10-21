package redact

import (
	"regexp"
	"strings"
)

// nextPair matches what follows a "," or ";" that ends a bare value: the
// next "key=" pair of a connection string, or a quote that closes it.
var nextPair = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_.]*=|["'])`)

// splitSecret cuts a bare value at the first "," or ";" that ends it: at
// the end of the value, or before the next pair or closing quote. A "," or
// ";" inside the value belongs to it, so "a,b;c" is one password.
func splitSecret(value string) (secret, tail string) {
	for i := 0; i < len(value); i++ {
		if value[i] != ',' && value[i] != ';' {
			continue
		}
		rest := strings.TrimLeft(value[i:], ",;")
		if rest == "" || nextPair.MatchString(rest) {
			return value[:i], value[i:]
		}
	}
	return value, ""
}

// rebuildKeyValue keeps the key and its separator, replaces the value and
// keeps what follows a bare value (see splitSecret). groups[1] is the key
// with its separator and groups[2] the value.
func rebuildKeyValue(groups []string) string {
	value := groups[2]
	if value == "" || value[0] == '"' || value[0] == '\'' {
		return groups[1] + redacted
	}
	_, tail := splitSecret(value)
	return groups[1] + redacted + tail
}
