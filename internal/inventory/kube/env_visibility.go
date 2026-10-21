package kube

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A literal value in a pod template is shown in a change only when its
// name says it is harmless and its shape looks harmless. Everything else
// is reported as "changed": a credential can hide under any name (DB_PWD,
// SALT, WEBHOOK_URL), so no list of secret-sounding names is enough.

// visibleNameTokens are the words of a variable or flag name whose value
// is shown: DB_HOST, LOG_LEVEL, --listen-addr.
var visibleNameTokens = map[string]bool{
	"HOST": true, "PORT": true, "URL": true, "LEVEL": true, "ENV": true,
	"REGION": true, "ENABLED": true, "TIMEOUT": true, "REPLICAS": true,
	"MODE": true, "ADDR": true,
}

// secretNameWords make a name hidden even next to a visible token
// (WEBHOOK_URL, SECRET_HOST).
var secretNameWords = []string{
	"password", "passwd", "pass", "pwd", "secret", "token", "key",
	"credential", "cred", "auth", "cert", "dsn", "hook", "salt", "sign",
	"bearer", "private", "cookie", "session",
}

// plainValue is the shape of a value that cannot be a credential in
// disguise: a short word, number, duration, host or path.
var plainValue = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,64}$`)

// hostOnly is a host with an optional scheme and port, and nothing after
// it: no user, no path (a path can hold a secret), no query.
var hostOnly = regexp.MustCompile(
	`^(?:[a-z][a-z0-9+.-]*://)?[A-Za-z0-9.-]+(?::[0-9]{1,5})?/?$`)

// nameTokens splits a name into upper-case words at punctuation and at
// camelCase boundaries: "dbHost" is DB and HOST.
func nameTokens(name string) []string {
	var tokens []string
	var word []rune
	flush := func() {
		if len(word) > 0 {
			tokens = append(tokens, strings.ToUpper(string(word)))
			word = nil
		}
	}
	for _, r := range name {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
		case unicode.IsUpper(r) && len(word) > 0 &&
			unicode.IsLower(word[len(word)-1]):
			flush()
			word = append(word, r)
		default:
			word = append(word, r)
		}
	}
	flush()
	return tokens
}

// envValueVisible reports whether the literal value of the named
// variable (or flag) may appear in a message.
func envValueVisible(name, value string) bool {
	lower := strings.ToLower(name)
	for _, word := range secretNameWords {
		if strings.Contains(lower, word) {
			return false
		}
	}
	named, url := false, false
	for _, token := range nameTokens(name) {
		named = named || visibleNameTokens[token]
		url = url || token == "URL"
	}
	if !named || !plainValue.MatchString(value) {
		return false
	}
	if url && !strings.Contains(value, "://") {
		// A URL variable holds an address: anything else is a secret.
		return false
	}
	return !strings.Contains(value, "/") || hostOnly.MatchString(value)
}

// shownValue is value for a message, or inventory.HiddenValue when it may
// not be shown.
func shownValue(name, value string) string {
	if value == "" || envValueVisible(name, value) {
		return value
	}
	return inventory.HiddenValue
}

// safeArgs writes a command line for a message: every value that follows
// a flag is hidden unless the flag's name allows it, and so is every
// attached value (-phunter2). Words that are not flag values stay when
// they are plain, except the word after a credential word ("password
// hunter2") and words that look like tokens.
func safeArgs(args []string) string {
	var out []string
	afterSecret := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !isFlag(arg) {
			out = append(out, positionalWord(arg, afterSecret))
			afterSecret = announcesSecret(arg)
			continue
		}
		afterSecret = false
		name, value, attached := strings.Cut(arg, "=")
		if attached {
			out = append(out, name+"="+shownValue(name, value))
			continue
		}
		if attachedShort(arg) {
			out = append(out, arg[:2]+inventory.HiddenValue)
			continue
		}
		out = append(out, arg)
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			out = append(out, shownValue(arg, args[i]))
		}
	}
	return strings.Join(out, " ")
}

func isFlag(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-" && arg != "--"
}

// positionalWord is a word that is not a flag value: hidden when a
// credential word came just before it.
func positionalWord(word string, afterSecret bool) string {
	if afterSecret && word != "" {
		return inventory.HiddenValue
	}
	return safeWord(word)
}

// attachedShort reports a single-dash flag longer than one letter whose
// name does not allow a value: "-phunter2" may be a flag with its value
// glued on.
func attachedShort(arg string) bool {
	if strings.HasPrefix(arg, "--") || len(arg) <= 2 {
		return false
	}
	return !envValueVisible(arg, "x")
}

// safeWord is a word of a command line that is not a flag value: plain
// words and paths stay, an address only as host and port.
func safeWord(word string) string {
	switch {
	case word == "" || word == "-" || word == "--":
		return word
	case strings.Contains(word, "://"):
		if hostOnly.MatchString(word) {
			return word
		}
		return inventory.HiddenValue
	case plainValue.MatchString(word):
		return hideTokenSegments(word)
	}
	return inventory.HiddenValue
}

// safePath is a probe path without its query, which can carry a key, and
// without segments that look like a token ("/hooks/<token>").
func safePath(path string) string {
	before, _, found := strings.Cut(path, "?")
	before = hideTokenSegments(before)
	if found {
		return before + "?" + inventory.HiddenValue
	}
	return before
}
