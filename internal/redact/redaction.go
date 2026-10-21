package redact

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/metrics"
)

// redactionRule replaces a match with placeholder. The first capture group,
// when present, is the non-secret prefix that stays visible. skip, when
// set, leaves a match unchanged; it receives the rule's capture groups.
type redactionRule struct {
	pattern     *regexp.Regexp
	placeholder string
	suffix      string
	skip        func(groups []string) bool
	// rebuild, when set, builds the whole replacement from the capture
	// groups instead of prefix + placeholder + suffix.
	rebuild func(groups []string) string
	// needs, when set, lists lower-case fragments of which at least one
	// must be in the text for the rule to be tried: a cheap check that
	// lets the key-name rules skip the lines that name no secret.
	needs []string
}

// secretNeeds are the fragments every key-name rule needs: each key word
// of secretKeys and camelSecretKeys contains one of them.
var secretNeeds = []string{
	"pass", "pwd", "token", "secret", "key", "credential", "sig", "session",
}

// wanted reports whether the rule can match text, given its lower-case
// form.
func (r redactionRule) wanted(lower string) bool {
	if len(r.needs) == 0 {
		return true
	}
	for _, fragment := range r.needs {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

const secretKeys = `password|passwd|passwort|sslpassword|pwd|passphrase|` +
	`passcode|token|secret|api[_-]?key|access[_-]?token|client[_-]?secret|` +
	`secret[_-]?access[_-]?key|private[_-]?key|credentials?|` +
	`access[_-]?key|account[_-]?key|signing[_-]?key|secret[_-]?key|` +
	`signature|session[_-]?id|pass`

// capsSecretKeys are the key words that follow an upper-case run with no
// separator: PGPASSWORD, DBPASSWORD, GITHUBTOKEN. Only these words, and
// only in capitals, so that "bypass" and "compass" stay words.
const capsSecretKeys = `(?-i:[A-Z]{2,})(?:password|passwd|passphrase|token|` +
	`secret|credentials?)`

// camelSecretKeys are the camelCase forms of key names: authToken,
// refreshToken, clientSecret, signingKey. The capital letter marks where
// the secret word starts, so "authtoken" or "tokenizer" are not keys.
const camelSecretKeys = `[a-z][a-z0-9]*(?-i:Token|Secret|Password|Passwd|` +
	`Passphrase|ApiKey|APIKey|SigningKey|AccessKey|PrivateKey|SecretKey|` +
	`Credentials?)`

// secretKey also matches environment-style names such as DB_PASSWORD,
// GITHUB_TOKEN or MY_API_KEY, plurals such as GITHUB_TOKENS, camelCase
// names such as authToken and npm's _authToken. The extra prefix and
// suffix of a snake_case name must be joined with "_" or "-", so ordinary
// words like "tokenizer" stay visible. Keys that only describe a secret,
// such as token_ttl or secret_name, are left alone by skipDescriptiveKey.
const secretKey = `(?:(?:[a-z0-9_-]*[_-])?(?:` + camelSecretKeys + `|` +
	secretKeys + `)s?(?:[_-][a-z0-9_-]*)?|` + capsSecretKeys +
	`s?(?:[_-][a-z0-9_-]*)?)`

// secretValue is a quoted value, which may contain spaces, or one bare
// word. A bare word runs to the next space: "a,b;c" is one password, and
// splitSecret gives back what follows when a "," or ";" ends the value.
//
// A quote that is never closed (a truncated log line) takes the rest of
// its line: the value is secret to its end, not only to the first space.
const secretValue = `(?:"(?:[^"\\]|\\.)*"|"[^\n]*|'[^']*'|'[^\n]*|[^\s]+)`

// jsonSecretValue is a JSON string, number or boolean. A string cut off
// before its closing quote runs to the end of its line.
const jsonSecretValue = `(?:"(?:[^"\\]|\\.)*"|"[^\n]*|` +
	`-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false)`

var credentialRedactions = []redactionRule{
	{pattern: regexp.MustCompile(
		`-----BEGIN [A-Z ]*PRIVATE KEY(?: BLOCK)?-----[\s\S]*?` +
			`(?:-----END [A-Z ]*PRIVATE KEY(?: BLOCK)?-----|$)`,
	), placeholder: "[redacted private key]"},
	// The value must not start with "[" so an already redacted header is
	// not counted twice.
	{pattern: regexp.MustCompile(
		`(?i)(\bbearer(?:\s*[:=]\s*|\s+))[^\s,;"\[][^\s,;"]*`,
	), placeholder: "[redacted]"},
	// A bare "basic" is a common English word, so only a value that is
	// base64 of "user:password" is treated as a credential.
	{pattern: regexp.MustCompile(`(?i)(\bbasic\s+)([A-Za-z0-9+/]+={0,2})`),
		placeholder: "[redacted]", skip: skipNonBasicCredential},
	{pattern: regexp.MustCompile(
		`(?i)([?&]` + secretKey + `=)([^&\s]+)`,
	), placeholder: "[redacted]", skip: skipDescriptiveOrCount,
		needs: secretNeeds},
	{pattern: regexp.MustCompile(
		`(?i)("` + secretKey + `"\s*:\s*)(` + jsonSecretValue + `)`,
	), placeholder: `"[redacted]"`, skip: skipDescriptiveOrCount,
		needs: secretNeeds},
	{pattern: regexp.MustCompile(
		`(?i)(\b` + secretKey + `\s*[:=]\s*)(` + secretValue + `)`,
	), placeholder: "[redacted]", skip: skipNonSecret,
		rebuild: rebuildKeyValue, needs: secretNeeds},
	// Cookie headers carry session credentials; the whole value goes.
	{pattern: regexp.MustCompile(
		`(?i)(\b(?:set-)?cookie\s*:\s*)[^\r\n\[][^\r\n]*`,
	), placeholder: "[redacted]"},
	// A flag whose name ends in a secret word takes its value as the next
	// word: "--password hunter2".
	{pattern: regexp.MustCompile(
		`(?i)(\B--[a-z0-9-]*(?:password|passwd|pwd|pass|passphrase|secret|` +
			`token|api[_-]?key)\s+)[^\s\[-][^\s]*`,
	), placeholder: "[redacted]"},
	// "password hunter2" without a separator is also prose ("password
	// policy"), so only a credential-looking word is redacted.
	{pattern: regexp.MustCompile(
		`(?i)(\b(?:password|passwd|pwd|passphrase)\s+)` +
			`([^\s,;"'\[=:][^\s,;"']*)`,
	), placeholder: "[redacted]", skip: skipProseWord},
	// mysql -u root -pSECRET: the password is glued to -p.
	{pattern: regexp.MustCompile(
		`(\bmysql[a-z]*\b[^\n|;&]*?\s-p)[^\s\[]\S*`,
	), placeholder: "[redacted]"},
	// Userinfo runs to the last "@" of the token, so a password containing
	// "@" or "/" does not leak its tail. The user may be empty, as in
	// redis://:password@host. A password with "?" or "#" has no "/" before
	// its "@"; one with "/" stops at a "?" or "#", which keeps
	// "http://host:80/x?mail=a@b.c" from being mistaken for userinfo.
	{pattern: regexp.MustCompile(
		`(?i)(\b[a-z][a-z0-9+.-]*://)[^/\s:@?#]*:` +
			`(?:[^\s?#]*|[^\s/]*)@`,
	), placeholder: "[redacted]", suffix: "@"},
	{pattern: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		placeholder: "[redacted]"},
	{pattern: regexp.MustCompile(
		`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+`,
	), placeholder: "[redacted]"},
	{pattern: bareTokenPattern, placeholder: "[redacted]"},
}

var privateAddressRedactions = []redactionRule{
	{pattern: regexp.MustCompile(`\b10\.(?:\d{1,3}\.){2}\d{1,3}\b`)},
	{pattern: regexp.MustCompile(`\b192\.168\.(?:\d{1,3}\.)\d{1,3}\b`)},
	{pattern: regexp.MustCompile(
		`\b172\.(?:1[6-9]|2\d|3[01])\.(?:\d{1,3}\.)\d{1,3}\b`,
	)},
	{pattern: regexp.MustCompile(
		`(?i)\bf[cd][0-9a-f]{2}:[0-9a-f:]*[0-9a-f]`,
	)},
	{pattern: regexp.MustCompile(`(?i)\bfe80:[0-9a-f:]*[0-9a-f]`)},
	// Link-local (the cloud metadata address) and shared address space
	// (100.64.0.0/10, carrier-grade NAT, used by some clusters).
	{pattern: regexp.MustCompile(`\b169\.254\.\d{1,3}\.\d{1,3}\b`)},
	{pattern: regexp.MustCompile(
		`\b100\.(?:6[4-9]|[7-9]\d|1[01]\d|12[0-7])\.\d{1,3}\.\d{1,3}\b`,
	)},
}

// Evidence removes credentials and private application addresses at
// the shared rendering boundary. Kubernetes identity fields are handled by
// the report separately and are intentionally not passed through here.
func Evidence(value string) string {
	return EvidenceWithPolicy(value, false)
}

// EvidenceWithPolicy keeps credentials redacted and optionally keeps
// private addresses visible when an operator explicitly opts in.
func EvidenceWithPolicy(
	value string, includePrivateAddresses bool,
) string {
	lower := strings.ToLower(value)
	for _, rule := range allCredentialRules {
		if rule.wanted(lower) {
			value = applyRedaction(value, rule)
		}
	}
	if !includePrivateAddresses {
		for _, rule := range privateAddressRedactions {
			rule.placeholder = "[private-address]"
			value = applyRedaction(value, rule)
		}
	}
	return value
}

func applyRedaction(value string, rule redactionRule) string {
	return rule.pattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := rule.pattern.FindStringSubmatch(match)
		if rule.skip != nil && rule.skip(groups) {
			return match
		}
		metrics.DefaultRegistry().RedactedValues.Add(1)
		if rule.rebuild != nil {
			return rule.rebuild(groups)
		}
		keep := ""
		if len(groups) > 1 {
			keep = groups[1]
		}
		return keep + rule.placeholder + rule.suffix
	})
}

// Credentials removes credentials only. It is used at ingest, where
// private-address policy is still decided later by the renderer.
func Credentials(value string) string {
	return EvidenceWithPolicy(value, true)
}
