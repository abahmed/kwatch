package message

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/metrics"
)

var credentialRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)(basic\s+)[^\s,;]+`),
	regexp.MustCompile(
		`(?i)([?&](?:token|secret|password|api[_-]?key|` +
			`access[_-]?token)=)[^&\s]+`,
	),
	regexp.MustCompile(
		`(?i)\b(?:password|passwd|token|secret|api[_-]?key|` +
			`access[_-]?token)\s*[:=]\s*[^\s,;]+`,
	),
}

var privateAddressRedactions = []*regexp.Regexp{
	regexp.MustCompile(`\b10\.(?:\d{1,3}\.){2}\d{1,3}\b`),
	regexp.MustCompile(`\b192\.168\.(?:\d{1,3}\.)\d{1,3}\b`),
	regexp.MustCompile(`\b172\.(?:1[6-9]|2\d|3[01])\.(?:\d{1,3}\.)\d{1,3}\b`),
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
	patterns := credentialRedactions
	if !includePrivateAddresses {
		patterns = append(patterns, privateAddressRedactions...)
	}
	for _, pattern := range patterns {
		value = pattern.ReplaceAllStringFunc(value, func(match string) string {
			metrics.DefaultRegistry().RedactedValues.Add(1)
			if len(match) > 0 && (match[0] == '?' || match[0] == '&') {
				for i, r := range match {
					if r == '=' {
						return match[:i+1] + "[redacted]"
					}
				}
			}
			if len(match) >= 7 && match[:7] == "Bearer " {
				return "Bearer [redacted]"
			}
			if len(match) >= 6 && match[:6] == "Basic " {
				return "Basic [redacted]"
			}
			if len(match) > 0 && match[0] >= '0' && match[0] <= '9' {
				return "[private-address]"
			}
			for i, r := range match {
				if r == ':' || r == '=' {
					return match[:i+1] + "[redacted]"
				}
			}
			return "[redacted]"
		})
	}
	return value
}
