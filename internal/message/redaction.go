package message

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/metrics"
)

// redactionRule replaces a match with placeholder. The first capture group,
// when present, is the non-secret prefix that stays visible.
type redactionRule struct {
	pattern     *regexp.Regexp
	placeholder string
	suffix      string
}

const secretKeys = `password|passwd|token|secret|api[_-]?key|` +
	`access[_-]?token|client[_-]?secret`

var credentialRedactions = []redactionRule{
	{pattern: regexp.MustCompile(
		`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?` +
			`(?:-----END [A-Z ]*PRIVATE KEY-----|$)`,
	), placeholder: "[redacted private key]"},
	{pattern: regexp.MustCompile(`(?i)(bearer\s+)[^\s,;"]+`),
		placeholder: "[redacted]"},
	{pattern: regexp.MustCompile(`(?i)(basic\s+)[^\s,;"]+`),
		placeholder: "[redacted]"},
	{pattern: regexp.MustCompile(
		`(?i)([?&](?:` + secretKeys + `)=)[^&\s]+`,
	), placeholder: "[redacted]"},
	{pattern: regexp.MustCompile(
		`(?i)("(?:` + secretKeys + `)"\s*:\s*)"[^"]*"`,
	), placeholder: `"[redacted]"`},
	{pattern: regexp.MustCompile(
		`(?i)(\b(?:` + secretKeys + `)\s*[:=]\s*)[^\s,;]+`,
	), placeholder: "[redacted]"},
	{pattern: regexp.MustCompile(
		`(?i)(\b[a-z][a-z0-9+.-]*://)[^/\s:@]+:[^/\s@]+@`,
	), placeholder: "[redacted]", suffix: "@"},
	{pattern: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		placeholder: "[redacted]"},
	{pattern: regexp.MustCompile(
		`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+`,
	), placeholder: "[redacted]"},
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
}

// RedactEvidence removes credentials and private application addresses at
// the shared rendering boundary. Kubernetes identity fields are handled by
// the report separately and are intentionally not passed through here.
func RedactEvidence(value string) string {
	return RedactEvidenceWithPolicy(value, false)
}

// RedactEvidenceWithPolicy keeps credentials redacted and optionally keeps
// private addresses visible when an operator explicitly opts in.
func RedactEvidenceWithPolicy(
	value string, includePrivateAddresses bool,
) string {
	for _, rule := range credentialRedactions {
		value = applyRedaction(value, rule)
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
		metrics.DefaultRegistry().RedactedValues.Add(1)
		keep := ""
		if groups := rule.pattern.FindStringSubmatch(match); len(groups) > 1 {
			keep = groups[1]
		}
		return keep + rule.placeholder + rule.suffix
	})
}
