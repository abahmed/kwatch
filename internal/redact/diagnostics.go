package redact

import "regexp"

// diagnosticsRedactions widen the credential pass for text that leaves
// the cluster in a support bundle: every URL, e-mail address and IP
// address goes, not only private ones. Order matters: URLs first, so an
// address inside a URL is removed with it.
var diagnosticsRedactions = []redactionRule{
	{
		pattern:     regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`),
		placeholder: "<url>",
	},
	{
		pattern: regexp.MustCompile(
			`(?i)\b[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}\b`),
		placeholder: "<email>",
	},
	{
		pattern: regexp.MustCompile(
			`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
		placeholder: "<ip>",
	},
	{
		// IPv6: at least three groups, or a :: abbreviation, so clock
		// times such as 12:30:45 never match.
		pattern: regexp.MustCompile(
			`(?i)\b(?:[0-9a-f]{1,4}:){2,7}[0-9a-f]{1,4}\b|` +
				`(?i)\b(?:[0-9a-f]{1,4}:)+:(?:[0-9a-f]{1,4}` +
				`(?::[0-9a-f]{1,4})*)?`),
		placeholder: "<ip>",
		skip:        func(g []string) bool { return isClockTime(g[0]) },
	},
}

var clockTime = regexp.MustCompile(`^\d{1,2}:\d{2}(?::\d{2})?$`)

func isClockTime(s string) bool { return clockTime.MatchString(s) }

// Diagnostics is the single redaction list for the installer's
// diagnostics bundle (`kwatch redact`): credentials, then every URL,
// e-mail address and IP address.
func Diagnostics(value string) string {
	value = Credentials(value)
	for _, rule := range diagnosticsRedactions {
		value = applyRedaction(value, rule)
	}
	return value
}
