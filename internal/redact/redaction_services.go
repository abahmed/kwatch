package redact

import (
	"regexp"
	"strings"
)

// serviceRedactions cover credential shapes of cloud services and
// registries that carry no obvious "key=value" form.
var serviceRedactions = []redactionRule{
	// Digest authentication: only the response proves the password.
	{pattern: regexp.MustCompile(
		`(?i)(\bdigest\b[^\n]*?\bresponse\s*=\s*\\?")[^"\\\n]+`,
	), placeholder: redacted, needs: []string{"digest"}},
	// A bare token as the whole user of a web address:
	// https://TOKEN@host. The user of ssh or git addresses is a name.
	{pattern: regexp.MustCompile(`(?i)(\bhttps?://)[^/\s:@?#\[]+@`),
		placeholder: redacted, suffix: "@", needs: []string{"@"}},
	// A shared access signature in a query: ?sv=2020&sig=...
	{pattern: regexp.MustCompile(`(?i)([?&]sig=)[^&\s]+`),
		placeholder: redacted, needs: []string{"sig"}},
	// docker config: "auth": "<base64 of user:password>", auth=<base64>.
	{pattern: regexp.MustCompile(
		`(?i)("auth"\s*:\s*\\?")[A-Za-z0-9+/=]{8,}`,
	), placeholder: redacted, needs: []string{"auth"}},
	{pattern: regexp.MustCompile(
		`(?i)(\bauth\s*=\s*)([A-Za-z0-9+/]{12,}={0,2})`,
	), placeholder: redacted, skip: skipNonBasicCredential,
		needs: []string{"auth"}},
	{pattern: regexp.MustCompile(
		`(?i)(\bdockerconfigjson"?\s*[:=]\s*"?)[A-Za-z0-9+/=_-]{16,}`,
	), placeholder: redacted, needs: []string{"dockerconfigjson"}},
	// A base64 JSON document ({" is eyJ): a registry login file, a token.
	{pattern: regexp.MustCompile(`\beyJ[A-Za-z0-9+/_-]{60,}={0,2}`),
		placeholder: redacted, needs: []string{"eyj"}},
	// "the password is X" says the secret in prose; a word that only
	// describes the password ("is required") stays.
	{pattern: regexp.MustCompile(
		`(?i)(\b(?:password|passwd|passphrase|passcode)\s+(?:is|was)\s+)` +
			`([^\s,;"'\[]+)`,
	), placeholder: redacted, skip: skipDescribingWord,
		needs: []string{"pass"}},
}

// describingWords follow "password is" when they describe the password
// rather than give it.
var describingWords = map[string]bool{
	"required": true, "invalid": true, "wrong": true, "incorrect": true,
	"empty": true, "missing": true, "expired": true, "too": true,
	"not": true, "blank": true, "unset": true, "set": true, "a": true,
	"an": true, "the": true, "rejected": true, "different": true,
	"same": true, "changed": true, "correct": true, "weak": true,
	"stored": true, "hashed": true, "encrypted": true, "ignored": true,
}

// skipDescribingWord keeps the word after "password is" when it only
// describes the password. groups[2] is the word.
func skipDescribingWord(groups []string) bool {
	if len(groups) < 3 {
		return true
	}
	word := strings.ToLower(strings.TrimRight(groups[2], ".!?"))
	return describingWords[word] || word == redacted
}
