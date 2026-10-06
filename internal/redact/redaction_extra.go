package redact

import (
	"regexp"
	"strings"
)

// allCredentialRules is every credential rule in the order it runs. The
// extra rules go first: they match shapes (headers, escaped JSON, webhook
// URLs) that the general key/value rules would otherwise only half cover.
var allCredentialRules = append(
	append([]redactionRule{}, extraRedactions...), credentialRedactions...,
)

const redacted = "[redacted]"

// extraRedactions covers credential shapes that carry no obvious
// "key=value" form.
var extraRedactions = []redactionRule{
	// AWS secret access keys have no marker of their own, but one that
	// sits right after an access key id is the matching secret.
	{pattern: regexp.MustCompile(
		`(\b(?:AKIA|ASIA)[0-9A-Z]{16}[ \t:,;="']{1,3})[A-Za-z0-9/+=]{40,}`,
	), placeholder: redacted},
	// An Authorization header always carries a credential, whatever the
	// scheme. Groups: 1 head, 2 optional scheme, 3 value, 4 a marker that
	// shows the value was already redacted.
	{pattern: regexp.MustCompile(
		`(?i)(\bauthorization(?:\\?")?[ \t]*[:=][ \t]*(?:\\?")?)` +
			`([A-Za-z][A-Za-z0-9_-]*[ \t]+)?` +
			`([^\s,;"\\\[][^\s,;"\\]*)(\s+\[redacted\])?`,
	), skip: skipHarmlessAuthorization,
		rebuild: func(g []string) string { return g[1] + g[2] + redacted }},
	// The same shape inside a JSON string that was itself escaped.
	{pattern: regexp.MustCompile(
		`(?i)(\\"` + secretKey + `\\"\s*:\s*)` +
			`(\\"(?:[^"\\]|\\[^"])*\\"|-?\d+(?:\.\d+)?|true|false)`,
	), placeholder: `\"` + redacted + `\"`, skip: skipDescriptiveOrCount,
		needs: secretNeeds},
	// Kubernetes env entries put the name and the value on separate
	// lines: "name: DB_PASSWORD" then "value: x". Groups: 1 everything up
	// to the value, 2 the variable name.
	{pattern: regexp.MustCompile(
		`(?i)(\bname"?\s*:\s*["']?(` + secretKey + `)["']?\s*,?\s*` +
			`"?value"?\s*:\s*)` + secretValue,
	), skip: skipDescriptiveEnvName, rebuild: rebuildEnvValue,
		needs: secretNeeds},
	// PINs are short digit strings, so only a number is redacted: "pin:
	// node-1" and "pin the version" stay visible. Groups: 1 key and
	// separator (and an opening quote), 2 the digits.
	{pattern: regexp.MustCompile(
		`(?i)(\b(?:[a-z0-9]+[_-])*pin(?:code)?"?\s*[:=]\s*["']?)\d{3,}`,
	), placeholder: redacted, needs: []string{"pin"}},
	// A bare "key=" is too common to redact blindly, so only a value that
	// looks like a credential and has six or more characters goes. A
	// key that continues a word, a dotted name or a snake_case name is
	// not matched here; secret_key and api_key have their own rules.
	{pattern: regexp.MustCompile(
		`(?i)((?:^|[^a-z0-9_.-])key\s*=\s*)(` + secretValue + `)`,
	), placeholder: redacted, skip: skipShortOrPlainKeyValue,
		needs: []string{"key"}},
	{pattern: regexp.MustCompile(
		`(?i)(\bhooks\.slack\.com/services/)[A-Za-z0-9/_-]+`,
	), placeholder: redacted},
	{pattern: regexp.MustCompile(
		`(?i)(\bdiscord(?:app)?\.com/api/(?:v\d+/)?webhooks/)` +
			`[A-Za-z0-9._/-]+`,
	), placeholder: redacted},
	// Telegram puts the bot token in the URL path: /bot<id>:<token>/.
	{pattern: regexp.MustCompile(`(\bbot)\d{6,}:[A-Za-z0-9_-]{30,}`),
		placeholder: redacted},
	// Go MySQL-style DSNs: user:pass@tcp(host:3306)/db. Groups: 1 the
	// user, 2 the network part that follows the "@".
	{pattern: regexp.MustCompile(
		`\b([A-Za-z0-9_.-]+:)[^\s'"]*@((?:tcp|udp|unix)\()`,
	), rebuild: func(g []string) string {
		return g[1] + redacted + "@" + g[2]
	}},
	// "apikey Zx81Qw9Lm2" without a separator is also prose ("api key
	// not found"), so only a credential-looking word is redacted.
	{pattern: regexp.MustCompile(
		`(?i)(\b(?:api[_ -]?key|access[_ -]?key|secret[_ -]?key|` +
			`auth[_ -]?token)[ \t]+)([^\s,;"'\[=:][^\s,;"']*)`,
	), placeholder: redacted, skip: skipProseWord},
}

// knownAuthSchemes are the schemes whose value is always a credential.
var knownAuthSchemes = map[string]bool{
	"token": true, "bearer": true, "basic": true,
}

// skipHarmlessAuthorization leaves an Authorization match alone when it
// was already redacted, or when it is prose such as "authorization: failed
// for user" rather than a credential.
func skipHarmlessAuthorization(g []string) bool {
	if len(g) < 5 || g[4] != "" {
		return true
	}
	scheme := strings.ToLower(strings.TrimSpace(g[2]))
	if knownAuthSchemes[scheme] {
		return false
	}
	return !looksLikeCredential(g[3])
}

// skipDescriptiveEnvName keeps DB_PASSWORD_FILE-style names; groups[2] is
// the variable name.
func skipDescriptiveEnvName(g []string) bool {
	return len(g) > 2 && skipDescriptiveKey([]string{"", g[2]})
}

// rebuildEnvValue keeps the name line and swaps the value, quoting the
// placeholder when the value sat in a JSON string.
func rebuildEnvValue(g []string) string {
	if strings.Contains(g[1], `"value"`) {
		return g[1] + `"` + redacted + `"`
	}
	return g[1] + redacted
}

// skipShortOrPlainKeyValue keeps the value of a bare "key=" unless it has
// at least six characters and looks like a credential. groups[2] is the
// value.
func skipShortOrPlainKeyValue(g []string) bool {
	if len(g) < 3 {
		return true
	}
	value := strings.Trim(g[2], `"'`)
	return len(value) < 6 || !looksLikeCredential(value)
}
