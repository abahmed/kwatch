package enricher

import "regexp"

var signatureHints = []struct {
	re   *regexp.Regexp
	hint string
}{
	{regexp.MustCompile(`(?i)connection refused.*:5432|dial tcp.*:5432`), "Postgres unreachable — check the DB Service/endpoints + connection string."},
	{regexp.MustCompile(`(?i)x509|certificate (has expired|signed by unknown|verify failed)`), "TLS/cert issue — check the CA bundle and cert validity."},
	{regexp.MustCompile(`(?i)no space left on device`), "Disk full — check PVC / ephemeral-storage usage."},
	{regexp.MustCompile(`(?i)i/o timeout|context deadline exceeded|dial tcp .* timeout`), "Network/dependency timeout — check the target Service / NetworkPolicy."},
	{regexp.MustCompile(`(?i)permission denied|forbidden|unauthorized|RBAC`), "AuthZ/RBAC or filesystem-permission issue."},
	{regexp.MustCompile(`(?i)out of memory|cannot allocate memory`), "Memory exhaustion — raise limits or fix the leak."},
}

func signatureHint(logs string) string {
	for _, s := range signatureHints {
		if s.re.MatchString(logs) {
			return s.hint
		}
	}
	return ""
}
