package llm

import "regexp"

var defaultRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|secret|token|api[-_]?key|bearer)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)authorization:\s*\S+`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
}

type redactor struct{ patterns []*regexp.Regexp }

func newRedactor() *redactor { return &redactor{patterns: defaultRedactions} }

func (r *redactor) scrub(s string) string {
	for _, re := range r.patterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}
