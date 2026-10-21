package kube

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/abahmed/kwatch/internal/inventory"
)

// positionalSecretWords are the words of a command line that announce a
// credential as the next word: "mysqladmin password hunter2", "redis-cli
// auth hunter2". They are matched as whole words of the argument.
var positionalSecretWords = map[string]bool{
	"PASSWORD": true, "PASSWD": true, "PASS": true, "PWD": true,
	"SECRET": true, "TOKEN": true, "KEY": true, "AUTH": true,
	"CREDENTIAL": true, "CREDENTIALS": true, "CRED": true,
}

// announcesSecret reports a positional word that names a credential.
func announcesSecret(word string) bool {
	for _, token := range nameTokens(word) {
		if positionalSecretWords[token] {
			return true
		}
	}
	return false
}

var uuidShape = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-` +
		`[0-9a-fA-F]{12}$`)

// Parts of a name this long that mix letters and digits, or that switch
// case again and again, are random strings, not words.
const (
	tokenMixedLength = 12
	tokenCaseLength  = 20
	tokenCaseSwitch  = 4
)

// looksLikeToken reports a path segment or word that looks like a random
// credential: a hash, a UUID, a base64 string. Ordinary words, numbers and
// dates do not.
func looksLikeToken(segment string) bool {
	if uuidShape.MatchString(segment) {
		return true
	}
	parts := strings.FieldsFunc(segment, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	for _, part := range parts {
		if len(part) >= tokenMixedLength && mixesLettersAndDigits(part) ||
			len(part) >= tokenCaseLength &&
				caseSwitches(part) >= tokenCaseSwitch {
			return true
		}
	}
	return false
}

func mixesLettersAndDigits(part string) bool {
	letters, digits := false, false
	for _, r := range part {
		letters = letters || unicode.IsLetter(r)
		digits = digits || unicode.IsDigit(r)
	}
	return letters && digits
}

// caseSwitches counts the places where a lower-case letter is followed by
// an upper-case one.
func caseSwitches(part string) int {
	count, previousLower := 0, false
	for _, r := range part {
		if previousLower && unicode.IsUpper(r) {
			count++
		}
		previousLower = unicode.IsLower(r)
	}
	return count
}

// hideTokenSegments replaces every segment of a slash-separated word that
// looks like a token.
func hideTokenSegments(word string) string {
	segments := strings.Split(word, "/")
	for i, segment := range segments {
		if looksLikeToken(segment) {
			segments[i] = inventory.HiddenValue
		}
	}
	return strings.Join(segments, "/")
}
