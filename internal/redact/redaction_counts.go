package redact

import (
	"regexp"
	"strings"
)

// Configuration keys such as maxTokens=4096 count tokens, they do not hold
// one. A key is a count when its value is a plain number and either its
// last word is the plural "tokens" (authTokens, maxTokens) or a counting
// word comes before a last word "token" (numToken, inputToken). Only the
// token word is ever a count: password, secret, key and credential words
// never are. A key like authToken=1234 stays redacted: one token, not many.
// A number of more than seven digits is never a count.

// countWords are the words that make the secret word a quantity.
var countWords = map[string]bool{
	"max": true, "min": true, "num": true, "total": true, "limit": true,
	"avg": true, "prompt": true, "completion": true, "input": true,
	"output": true, "reserved": true, "budget": true,
}

var (
	numberValue = regexp.MustCompile(`^["']?\d{1,7}(?:\.\d+)?["']?$`)
	camelWord   = regexp.MustCompile(`[A-Z]?[a-z0-9]+|[A-Z]+`)
)

// skipDescriptiveOrCount keeps a key-value match whose key describes a
// secret (token_ttl) or counts it (maxTokens=4096). groups[1] is the key
// with its separator; groups[2], when the rule captures it, is the value.
func skipDescriptiveOrCount(groups []string) bool {
	if skipDescriptiveKey(groups) {
		return true
	}
	if len(groups) < 3 || !numberValue.MatchString(groups[2]) {
		return false
	}
	return isCountKey(keyName(groups[1]))
}

// keyName cuts the key out of a match prefix such as `"maxTokens": `.
func keyName(prefix string) string {
	key := strings.TrimLeft(prefix, `?&"\`)
	if end := strings.IndexFunc(key, notKeyChar); end >= 0 {
		key = key[:end]
	}
	return key
}

// isCountKey reports whether the key names a number of tokens.
func isCountKey(key string) bool {
	words := keyWords(key)
	if len(words) < 2 {
		return false
	}
	switch words[len(words)-1] {
	case "tokens":
		return true
	case "token":
		return countWords[words[len(words)-2]]
	}
	return false
}

// keyWords splits a snake_case, kebab-case or camelCase key into lower-case
// words.
func keyWords(key string) []string {
	var words []string
	for _, part := range strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-'
	}) {
		for _, w := range camelWord.FindAllString(part, -1) {
			words = append(words, strings.ToLower(w))
		}
	}
	return words
}
