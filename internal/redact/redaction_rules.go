package redact

import (
	"encoding/base64"
	"regexp"
	"strings"
)

// bareTokenPrefixes are well-known credential formats that are secret on
// their own, without a key such as "token=" in front of them. Minimum
// lengths keep short words like "sk-learn" visible.
var bareTokenPrefixes = []string{
	`gh[pousr]_[A-Za-z0-9]{20,}`,                 // GitHub tokens
	`github_pat_[A-Za-z0-9_]{20,}`,               // GitHub fine-grained tokens
	`xox[abpr]-[A-Za-z0-9-]{10,}`,                // Slack tokens
	`glpat-[A-Za-z0-9_-]{20,}`,                   // GitLab personal tokens
	`AIza[0-9A-Za-z_-]{35}`,                      // Google API keys
	`sk_(?:live|test)_[0-9A-Za-z]{16,}`,          // Stripe secret keys
	`sk-[A-Za-z0-9_-]{20,}`,                      // OpenAI-style keys
	`ya29\.[A-Za-z0-9_-]{20,}`,                   // Google OAuth access tokens
	`npm_[A-Za-z0-9]{30,}`,                       // npm access tokens
	`SG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}`, // SendGrid keys
}

var bareTokenPattern = regexp.MustCompile(
	`\b(?:` + strings.Join(bareTokenPrefixes, "|") + `)`,
)

// descriptiveSuffixes end key names that describe a secret without holding
// it, such as token_ttl, secret_name or DB_PASSWORD_FILE.
var descriptiveSuffixes = map[string]bool{
	"ttl": true, "name": true, "expiry": true, "ref": true, "id": true,
	"manager": true, "file": true, "path": true, "count": true,
	"seconds": true, "total": true, "size": true, "length": true,
	"type": true, "version": true,
}

// skipDescriptiveKey keeps a key-value match whose key ends with a
// descriptive suffix. groups[1] is the key with its separator, for example
// `token_ttl=`, `"secret_name": ` or `?token_id=`.
func skipDescriptiveKey(groups []string) bool {
	if len(groups) < 2 {
		return false
	}
	key := strings.TrimLeft(groups[1], `?&"\`)
	if end := strings.IndexFunc(key, notKeyChar); end >= 0 {
		key = key[:end]
	}
	parts := strings.FieldsFunc(strings.ToLower(key), func(r rune) bool {
		return r == '_' || r == '-'
	})
	return len(parts) > 0 && descriptiveSuffixes[parts[len(parts)-1]]
}

func notKeyChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return r != '_' && r != '-'
}

// skipNonBasicCredential keeps "basic <word>" unless the word is base64 of
// "user:password", the HTTP Basic credential format. groups[2] is the word.
func skipNonBasicCredential(groups []string) bool {
	if len(groups) < 3 {
		return true
	}
	decoded, err := base64.StdEncoding.DecodeString(groups[2])
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(groups[2])
	}
	return err != nil || !strings.Contains(string(decoded), ":")
}

// proseKeys are ordinary English nouns that often precede a colon in log
// lines ("failed to get secret: not found"). A bare, colon-separated value
// after them is redacted only when it looks like a credential.
var proseKeys = map[string]bool{
	"secret": true, "token": true, "credential": true,
}

// skipNonSecret combines the descriptive-key rule with the prose guards for
// the generic key/value rule. groups[1] is the key with its separator and
// groups[2] the value.
func skipNonSecret(groups []string) bool {
	if skipDescriptiveOrCount(groups) {
		return true
	}
	if len(groups) < 3 {
		return false
	}
	key := strings.ToLower(strings.TrimRight(groups[1], " \t:="))
	value := groups[2]
	if key == "pwd" {
		// PWD is the working directory environment variable.
		return strings.HasPrefix(value, "/") ||
			strings.HasPrefix(value, "~") || strings.HasPrefix(value, ".")
	}
	if !strings.Contains(groups[1], ":") || strings.Contains(groups[1], "=") {
		return false
	}
	if value == "" || value[0] == '"' || value[0] == '\'' {
		return false
	}
	return proseKeys[strings.TrimSuffix(key, "s")] &&
		!looksLikeCredential(value)
}

// skipProseWord keeps the word after "password" unless it looks like a
// credential. groups[2] is the word.
func skipProseWord(groups []string) bool {
	return len(groups) < 3 || !looksLikeCredential(groups[2])
}

// looksLikeCredential reports whether a bare word has the shape of a
// secret rather than of an English word: letters mixed with digits, a long
// run of digits, mixed case, base64 punctuation, hex, or a very long word.
func looksLikeCredential(word string) bool {
	var upper, lower, digit, symbol bool
	for _, r := range word {
		switch {
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= '0' && r <= '9':
			digit = true
		case r == '+' || r == '/' || r == '=' || r == '_':
			symbol = true
		}
	}
	letters := upper || lower
	switch {
	case digit && letters:
		return len(word) >= 4
	case digit:
		return len(word) >= 6
	case upper && lower:
		return len(word) >= 8
	case symbol && letters:
		return len(word) >= 8
	}
	return len(word) >= 24
}
